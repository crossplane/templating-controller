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

	kustomizeapi "sigs.k8s.io/kustomize/api/types"

	"gopkg.in/yaml.v3"

	"gopkg.in/alecthomas/kingpin.v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"

	stacksv1alpha1 "github.com/crossplaneio/resourcepacks/api/v1alpha1"
	"github.com/crossplaneio/resourcepacks/pkg/controllers"
	"github.com/crossplaneio/resourcepacks/pkg/operations/kustomize"
)

const (
	KustomizeEngine = "kustomize"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = stacksv1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var (
		// top level app definition
		app = kingpin.New(filepath.Base(os.Args[0]), "An open source multicloud control plane.").DefaultEnvars()
		//debug      = app.Flag("debug", "Run with debug logging.").Short('d').Bool()

		// The default controller mode.
		controllerCmd = app.Command(filepath.Base(os.Args[0]), "An open source multicloud control plane.").Default()

		// Configuration for the reconciler.
		reconcileCmd                = controllerCmd.Command("reconcile", "Reconcile a Custom Resource")
		templateStackNameInput      = controllerCmd.Flag("template-stack-name", "Name of the TemplateStack custom resource.").Required().String()
		templateStackNamespaceInput = controllerCmd.Flag("template-stack-namespace", "Namespace of the TemplateStack custom resource").String()
	)
	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	switch cmd {
	case reconcileCmd.FullCommand():
		ctx := context.Background()
		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme:         scheme,
			LeaderElection: true,
			Port:           9443,
		})
		kingpin.FatalIfError(err, "unable to start manager")

		ts := &stacksv1alpha1.TemplateStack{}
		key := types.NamespacedName{Name: *templateStackNameInput, Namespace: *templateStackNamespaceInput}
		kingpin.FatalIfError(mgr.GetClient().Get(ctx, key, ts), "could not fetch the TemplateStack object")

		var options []controllers.ResourcePackReconcilerOption
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

		controller := controllers.NewResourcePackReconciler(
			mgr,
			schema.FromAPIVersionAndKind(ts.Spec.Behavior.CRD.APIVersion, ts.Spec.Behavior.CRD.Kind),
			options...)
		kingpin.FatalIfError(
			ctrl.NewControllerManagedBy(mgr).
				For(&unstructured.Unstructured{}).
				Complete(controller),
			"could not create controller",
		)
		kingpin.FatalIfError(clientgoscheme.AddToScheme(scheme), "could not register client go scheme")
		kingpin.FatalIfError(stacksv1alpha1.AddToScheme(scheme), "could not register template stack scheme")
		kingpin.FatalIfError(mgr.Start(ctrl.SetupSignalHandler()), "unable to run the manager")
	}
}
