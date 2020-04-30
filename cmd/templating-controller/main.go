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
	"context"
	"os"
	"path/filepath"

	"gopkg.in/alecthomas/kingpin.v2"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	kustomizeapi "sigs.k8s.io/kustomize/api/types"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane/apis/stacks"
	"github.com/crossplane/crossplane/apis/stacks/v1alpha1"

	"github.com/crossplane/templating-controller/pkg/controllers"
	"github.com/crossplane/templating-controller/pkg/operations/helm3"
	"github.com/crossplane/templating-controller/pkg/operations/kustomize"
)

// Engine name constants.
const (
	KustomizeEngine = "kustomize"
	Helm3Engine     = "helm3"
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
	kingpin.MustParse(app.Parse(os.Args[1:]))
	sd := &v1alpha1.StackDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:      *stackDefinitionNameInput,
			Namespace: *stackDefinitionNamespaceInput,
		},
	}
	kingpin.FatalIfError(getStackDefinition(sd), "could not fetch the StackDefinition object")
	gvk := schema.FromAPIVersionAndKind(sd.Spec.Behavior.CRD.APIVersion, sd.Spec.Behavior.CRD.Kind)

	kingpin.FatalIfError(clientgoscheme.AddToScheme(scheme), "could not register client-go scheme")
	kingpin.FatalIfError(stacks.AddToScheme(scheme), "could not register stacks group scheme")

	mgrOptions := ctrl.Options{
		Scheme: scheme,
		Port:   9443,
	}
	// TODO(muvaf): This should be a flag but deployment generation happens in
	// unpack step which doesn't have information about namespace. So, we have to
	// fetch all this from StackDefinition's fields that are not part of behavior.
	if sd.Spec.PermissionScope == string(apiextensions.NamespaceScoped) {
		if mgrOptions.Namespace = sd.GetNamespace(); mgrOptions.Namespace == "" {
			kingpin.FatalUsage("Scope is chosen as %s but StackDefinition object does not have a namespace", sd.Spec.PermissionScope)
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOptions)
	kingpin.FatalIfError(err, "unable to start manager")

	zl := zap.New(zap.UseDevMode(*debugInput))
	if *debugInput {
		// The controller-runtime runs with a no-op logger by default. It is
		// *very* verbose even at info level, so we only provide it a real
		// logger when we're running in debug mode.
		ctrl.SetLogger(zl)
	}
	crLogger := logging.NewLogrLogger(zl.WithName(gvk.GroupKind().String()))

	options := []controllers.TemplatingReconcilerOption{
		controllers.WithLogger(crLogger),
	}
	switch sd.Spec.Behavior.Engine.Type {
	case KustomizeEngine:
		kustOpts := []kustomize.Option{kustomize.WithResourcePath(*resourceDirInput)}
		kustomization := &kustomizeapi.Kustomization{}
		if sd.Spec.Behavior.Engine.Kustomize != nil {
			kustOpts = append(kustOpts, kustomize.WithOverlayGenerator(kustomize.NewPatchOverlayGenerator(sd.Spec.Behavior.Engine.Kustomize.Overlays)))
			if sd.Spec.Behavior.Engine.Kustomize.Kustomization != nil {
				kingpin.FatalIfError(runtime.DefaultUnstructuredConverter.FromUnstructured(sd.Spec.Behavior.Engine.Kustomize.Kustomization.UnstructuredContent(), kustomization), "cannot unmarshal into kustomization object")
			}
		}
		options = append(options,
			controllers.WithTemplatingEngine(kustomize.NewKustomizeEngine(kustomization, kustOpts...)))
	case Helm3Engine:
		options = append(options,
			controllers.WithTemplatingEngine(helm3.NewHelm3Engine(
				helm3.WithResourcePath(*resourceDirInput),
				helm3.WithLogger(crLogger)),
			),
		)
	default:
		kingpin.FatalUsage("the engine type %s is not supported", sd.Spec.Behavior.Engine.Type)
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
// Instead, we use rest client to make one call directly for the time being.
func getStackDefinition(sd *v1alpha1.StackDefinition) error {
	config := ctrl.GetConfigOrDie()
	config.ContentConfig.GroupVersion = &v1alpha1.SchemeGroupVersion
	config.APIPath = "/apis"
	config.NegotiatedSerializer = serializer.NewCodecFactory(scheme)
	client, err := rest.RESTClientFor(config)
	if err != nil {
		return err
	}
	return client.Get().Name(sd.Name).Namespace(sd.Namespace).Resource("stackdefinitions").Do(context.Background()).Into(sd)
}
