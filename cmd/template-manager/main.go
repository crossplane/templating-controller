package template_manager

import (
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/crossplaneio/resourcepacks/pkg/controllers"

	"github.com/crossplaneio/crossplane-runtime/pkg/logging"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	"gopkg.in/alecthomas/kingpin.v2"
)

var scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
}

func main() {
	var (
		log = logging.Logger

		// top level app definition
		app = kingpin.New(filepath.Base(os.Args[0]), "An open source multicloud control plane.").DefaultEnvars()
		//debug      = app.Flag("debug", "Run with debug logging.").Short('d').Bool()

		// The default controller mode.
		controllerCmd = app.Command(filepath.Base(os.Args[0]), "An open source multicloud control plane.").Default()

		// Configuration for the reconciler.
		reconcileCmd = controllerCmd.Command("reconcile", "Reconcile a Custom Resource")
		gvkInput     = reconcileCmd.Flag("gvk", "GroupVersionKind information of the Custom Resource Definition to reconcile. The format is {kind}.{group}/{version}").HintOptions("examplekind.groupexample.crossplane.io/v1alpha1").Required().String()
		//templateStackNameInput      = reconcileCmd.Flag("template-stack-name", "Name of the TemplateStack custom resource.").Required().String()
		//templateStackNamespaceInput = reconcileCmd.Flag("template-stack-namespace", "Namespace of the TemplateStack custom resource").Required().String()
	)
	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	switch cmd {
	case reconcileCmd.FullCommand():
		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme:         scheme,
			LeaderElection: true,
			Port:           9443,
		})
		if err != nil {
			log.Error(err, "unable to start manager")
			os.Exit(1)
		}

		// todo: do this in a better way.
		gvkArr := strings.Split(*gvkInput, ".")
		kind := gvkArr[0]
		apiVersion := strings.Join(gvkArr[1:], ".")
		gvk := schema.FromAPIVersionAndKind(apiVersion, kind)

		controller := controllers.NewResourcePackReconciler(mgr, gvk)
		if err = ctrl.NewControllerManagedBy(mgr).
			For(&unstructured.Unstructured{}).
			Complete(controller); err != nil {
			log.Error(err, "could not create controller")
			os.Exit(1)
		}

		log.Info("starting manager")
		if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
			log.Error(err, "problem running manager")
			os.Exit(1)
		}
	}
}
