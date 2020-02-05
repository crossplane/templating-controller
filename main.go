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

	"github.com/crossplaneio/crossplane-runtime/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
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

	"github.com/crossplaneio/crossplane/apis/stacks"
	"github.com/crossplaneio/crossplane/apis/stacks/v1alpha1"

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

		stackDefinitionNameInput      = app.Flag("stack-definition-name", "Name of the StackDefinition custom resource.").Required().String()
		stackDefinitionNamespaceInput = app.Flag("stack-definition-namespace", "Namespace of the StackDefinition custom resource").String()
		resourceDirInput              = app.Flag("resources-dir", "Directory of the resources to be fetched as input to the templating engine").Required().ExistingDir()
		debugInput                    = app.Flag("debug", "Enable debug logging").Bool()
	)

	kingpin.FatalIfError(clientgoscheme.AddToScheme(scheme), "could not register client-go scheme")
	kingpin.FatalIfError(stacks.AddToScheme(scheme), "could not register stacks group scheme")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Port:   9443,
	})
	kingpin.FatalIfError(err, "unable to start manager")

	sd := &v1alpha1.StackDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:      *stackDefinitionNameInput,
			Namespace: *stackDefinitionNamespaceInput,
		},
	}
	kingpin.FatalIfError(getStackDefinition(sd), "could not fetch the StackDefinition object")
	gvk := schema.FromAPIVersionAndKind(sd.Spec.Behavior.CRD.APIVersion, sd.Spec.Behavior.CRD.Kind)

	zl := zap.New(zap.UseDevMode(*debugInput))
	if *debugInput {
		// The controller-runtime runs with a no-op logger by default. It is
		// *very* verbose even at info level, so we only provide it a real
		// logger when we're running in debug mode.
		ctrl.SetLogger(zl)
	}
	options := []controllers.TemplatingReconcilerOption{
		controllers.WithLogger(logging.NewLogrLogger(zl.WithName(gvk.GroupKind().String()))),
	}
	switch sd.Spec.Behavior.Engine.Type {
	case KustomizeEngine:
		// TODO(muvaf): investigate a better way to convert *Unstructured to *Kustomization.
		kustomizationYAML, err := yaml.Marshal(sd.Spec.Behavior.Engine.Kustomize.Kustomization)
		kingpin.FatalIfError(err, "cannot marshal kustomization object")
		kustomization := &kustomizeapi.Kustomization{}
		kingpin.FatalIfError(yaml.Unmarshal(kustomizationYAML, kustomization), "cannot unmarshal into kustomization object")
		options = append(options,
			controllers.WithTemplatingEngine(kustomize.NewKustomizeEngine(kustomization,
				kustomize.WithResourcePath(*resourceDirInput),
				kustomize.AdditionalOverlayGenerator(kustomize.NewPatchOverlayGenerator(sd.Spec.Behavior.Engine.Kustomize.Overlays)),
			)))
	}

	controller := controllers.NewTemplatingReconciler(mgr, gvk, options...)
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	kingpin.FatalIfError(
		ctrl.NewControllerManagedBy(mgr).
			For(u).
			Complete(controller),
		"could not create controller",
	)
	kingpin.FatalIfError(mgr.Start(ctrl.SetupSignalHandler()), "unable to run the manager")
}

// TODO: Controller-runtime client doesn't work until manager is started, which
// is a blocking operation. So, we can't call any controller-runtime client functions
// here in main.go
// Instead, we use rest client directly for the time being.
func getStackDefinition(sd *v1alpha1.StackDefinition) error {
	config := ctrl.GetConfigOrDie()
	config.ContentConfig.GroupVersion = &v1alpha1.SchemeGroupVersion
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.NewCodecFactory(scheme)
	client, err := rest.RESTClientFor(config)
	if err != nil {
		return err
	}
	return client.Get().Name(sd.Name).Namespace(sd.Namespace).Resource("stackdefinitions").Do().Into(sd)
}
