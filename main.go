/*
Copyright 2020 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"os"
	"path/filepath"

	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	kustomizeapi "sigs.k8s.io/kustomize/api/types"

	stacksv1alpha1 "github.com/crossplaneio/templating-controller/api/v1alpha1"
	"github.com/crossplaneio/templating-controller/pkg/controllers"
	"github.com/crossplaneio/templating-controller/pkg/operations/kustomize"
)

const (
	KustomizeEngine = "kustomize"
)

var (
	scheme = runtime.NewScheme()
)

func main() {
	var (
		// top level app definition
		app = kingpin.New(filepath.Base(os.Args[0]), "Templating controller for Crossplane Template Stacks.").DefaultEnvars()

		// The default controller mode.
		controllerCmd               = app.Command(filepath.Base(os.Args[0]), "Templating controller for Crossplane Template Stacks.").Default()
		templateStackNameInput      = controllerCmd.Flag("template-stack-name", "Name of the TemplateStack custom resource.").Required().String()
		templateStackNamespaceInput = controllerCmd.Flag("template-stack-namespace", "Namespace of the TemplateStack custom resource").String()
	)
	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	kingpin.FatalIfError(clientgoscheme.AddToScheme(scheme), "could not register client-go scheme")
	kingpin.FatalIfError(stacksv1alpha1.AddToScheme(scheme), "could not register stacks group scheme")

	switch cmd {
	case controllerCmd.FullCommand():
		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme: scheme,
			Port:   9443,
		})
		kingpin.FatalIfError(err, "unable to start manager")
		ts := &stacksv1alpha1.TemplateStack{
			ObjectMeta: v1.ObjectMeta{
				Name:      *templateStackNameInput,
				Namespace: *templateStackNamespaceInput,
			},
		}
		kingpin.FatalIfError(getTemplateStack(ts), "could not fetch the TemplateStack object")

		var options []controllers.TemplatingReconcilerOption
		switch ts.Spec.Behavior.EngineConfiguration.Type {
		case KustomizeEngine:
			// TODO(muvaf): investigate a better way to convert *Unstructured to *Kustomization.
			kustomizationYAML, err := yaml.Marshal(ts.Spec.Behavior.EngineConfiguration.Kustomization)
			kingpin.FatalIfError(err, "cannot marshal kustomization object")
			kustomization := &kustomizeapi.Kustomization{}
			kingpin.FatalIfError(yaml.Unmarshal(kustomizationYAML, kustomization), "cannot unmarshal into kustomization object")

			options = append(options, controllers.WithTemplatingEngine(kustomize.NewKustomizeEngine(kustomization,
				kustomize.WithResourcePath(ts.Spec.Behavior.Source.Path),
				kustomize.AdditionalOverlayGenerator(kustomize.NewPatchOverlayGenerator(ts.Spec.Behavior.EngineConfiguration.Overlays)),
			)))
		}

		gvk := schema.FromAPIVersionAndKind(ts.Spec.Behavior.CRD.APIVersion, ts.Spec.Behavior.CRD.Kind)
		controller := controllers.NewTemplatingReconciler(mgr, gvk, options...)
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(gvk)
		kingpin.FatalIfError(
			ctrl.NewControllerManagedBy(mgr).
				For(u).
				Complete(controller),
			"could not create controller",
		)
		kingpin.FatalIfError(clientgoscheme.AddToScheme(scheme), "could not register client go scheme")
		kingpin.FatalIfError(stacksv1alpha1.AddToScheme(scheme), "could not register template stack scheme")
		kingpin.FatalIfError(mgr.Start(ctrl.SetupSignalHandler()), "unable to run the manager")
	}
}

// TODO: Controller-runtime client doesn't work until manager is started, which
// is a blocking operation. So, we can't call any controller-runtime client functions
// here in main.go
// Instead, we use rest client directly for the time being.
func getTemplateStack(ts *stacksv1alpha1.TemplateStack) error {
	config := ctrl.GetConfigOrDie()
	config.ContentConfig.GroupVersion = &stacksv1alpha1.GroupVersion
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.NewCodecFactory(scheme)
	client, err := rest.RESTClientFor(config)
	if err != nil {
		return err
	}
	return client.Get().Name(ts.Name).Namespace(ts.Namespace).Resource("templatestacks").Do().Into(ts)
}
