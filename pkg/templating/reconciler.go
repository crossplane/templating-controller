/*
Copyright 2019 The Crossplane Authors.

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

package templating

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/net/context"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	runtimeresource "github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane/templating-controller/pkg/resource"
)

const (
	reconcileTimeout = 1 * time.Minute

	// TODO(muvaf): Once we get customizable exponential backoff, we should not
	// need this tinyWait.
	tinyWait = 1 * time.Second

	defaultShortWait = 30 * time.Second
	defaultLongWait  = 1 * time.Minute
	finalizer        = "templating-controller.crossplane.io"

	errUpdateResourceStatus  = "could not update status of the parent resource"
	errGetResource           = "could not get the parent resource"
	errTemplatingOperation   = "templating operation failed"
	errChildResourcePatchers = "child resource patchers failed"
	errDeleter               = "cannot run deleter"
	errAddFinalizer          = "cannot add finalizer to parent resource"
	errRemoveFinalizer       = "cannot remove finalizer from parent resource"
	errApply                 = "apply failed"
	errCreateChildResource   = "could not create child resource"
	errGetChildResource      = "could not get child resource"

	msgWaitingForDeletion = "waiting for deletion of child resources"
)

// ReconcilerOption is used to provide necessary changes to templating
// reconciler configuration.
type ReconcilerOption func(*Reconciler)

// WithChildResourcePatcher returns a ReconcilerOption that changes the
// ChildResourcePatchers.
func WithChildResourcePatcher(op ...ChildResourcePatcher) ReconcilerOption {
	return func(reconciler *Reconciler) {
		reconciler.children.ChildResourcePatcherChain = op
	}
}

// WithEngine returns a ReconcilerOption that changes the
// templating engine.
func WithEngine(eng Engine) ReconcilerOption {
	return func(reconciler *Reconciler) {
		reconciler.templating = eng
	}
}

// WithShortWait returns a ReconcilerOption that changes the wait
// duration that determines after how much time another reconcile should be triggered
// after an error pass.
func WithShortWait(d time.Duration) ReconcilerOption {
	return func(reconciler *Reconciler) {
		reconciler.shortWait = d
	}
}

// WithLongWait returns a ReconcilerOption that changes the wait
// duration that determines after how much time another reconcile should be triggered
// after a successful pass.
func WithLongWait(d time.Duration) ReconcilerOption {
	return func(reconciler *Reconciler) {
		reconciler.longWait = d
	}
}

// WithLogger returns a ReconcilerOption that changes the logger.
func WithLogger(l logging.Logger) ReconcilerOption {
	return func(reconciler *Reconciler) {
		reconciler.log = l
	}
}

func defaultCRChildren(c client.Client) crChildren {
	return crChildren{
		ChildResourcePatcherChain: ChildResourcePatcherChain{
			NewOwnerReferenceAdder(),
			NewDefaultingAnnotationRemover(),
			NewNamespacePatcher(),
			NewLabelPropagator(),
			NewParentLabelSetAdder(),
		},
		ChildResourceDeleter: NewAPIOrderedDeleter(c),
	}
}

type crChildren struct {
	ChildResourcePatcherChain
	ChildResourceDeleter
}

// NewReconciler returns a new templating reconciler that will reconcile
// given GroupVersionKind.
func NewReconciler(m manager.Manager, of schema.GroupVersionKind, options ...ReconcilerOption) *Reconciler {
	nr := func() resource.ParentResource {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(of)
		return u
	}

	r := &Reconciler{
		kube:              m.GetClient(),
		newParentResource: nr,
		shortWait:         defaultShortWait,
		longWait:          defaultLongWait,
		log:               logging.NewNopLogger(),
		templating:        &NopEngine{},
		finalizer:         runtimeresource.NewAPIFinalizer(m.GetClient(), finalizer),
		children:          defaultCRChildren(m.GetClient()),
	}

	for _, opt := range options {
		opt(r)
	}
	return r
}

// Reconciler is used to reconcile an arbitrary CRD whose GroupVersionKind
// is supplied.
type Reconciler struct {
	kube              client.Client
	newParentResource func() resource.ParentResource
	shortWait         time.Duration
	longWait          time.Duration
	log               logging.Logger

	templating Engine
	finalizer  runtimeresource.Finalizer
	children   crChildren
}

// Reconcile is called by controller-runtime for reconciliation.
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) { // nolint:gocyclo
	// NOTE(muvaf): This method is well over our cyclomatic complexity goal.
	// Be wary of adding additional complexity.

	ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
	defer cancel()
	log := r.log.WithValues("parent-resource", req)

	cr := r.newParentResource()
	if err := r.kube.Get(ctx, req.NamespacedName, cr); err != nil {
		// There's no need to requeue if the resource no longer exists. Otherwise
		// we'll be requeued implicitly because we return an error.
		log.Info("Cannot get the requested resource", "error", err)
		return reconcile.Result{Requeue: false}, errors.Wrap(client.IgnoreNotFound(err), errGetResource)
	}

	childResources, err := r.templating.Run(cr)
	if err != nil {
		log.Info("Cannot run templating operation", "error", err)
		omitError(log, resource.SetConditions(cr, v1alpha1.ReconcileError(errors.Wrap(err, errTemplatingOperation))))
		return ctrl.Result{RequeueAfter: r.shortWait}, errors.Wrap(r.kube.Status().Update(ctx, cr), errUpdateResourceStatus)
	}

	childResources, err = r.children.Patch(cr, childResources)
	if err != nil {
		log.Info("Cannot run patchers on the child resources", "error", err)
		omitError(log, resource.SetConditions(cr, v1alpha1.ReconcileError(errors.Wrap(err, errChildResourcePatchers))))
		return ctrl.Result{RequeueAfter: r.shortWait}, errors.Wrap(r.kube.Status().Update(ctx, cr), errUpdateResourceStatus)
	}

	if meta.WasDeleted(cr) {
		deleting, err := r.children.Delete(ctx, childResources)
		if err != nil {
			log.Info(errDeleter, "error", err)
			omitError(log, resource.SetConditions(cr, v1alpha1.ReconcileError(errors.Wrap(err, errDeleter))))
			return ctrl.Result{RequeueAfter: r.shortWait}, errors.Wrap(r.kube.Status().Update(ctx, cr), errUpdateResourceStatus)
		}

		if len(deleting) > 0 {
			omitError(log, resource.SetConditions(cr, v1alpha1.ReconcileSuccess().WithMessage(msgWaitingForDeletion)))
			return ctrl.Result{RequeueAfter: tinyWait}, errors.Wrap(r.kube.Status().Update(ctx, cr), errUpdateResourceStatus)
		}

		if err := r.finalizer.RemoveFinalizer(ctx, cr); client.IgnoreNotFound(err) != nil {
			log.Info(errRemoveFinalizer, "error", err)
			omitError(log, resource.SetConditions(cr, v1alpha1.ReconcileError(errors.Wrap(err, errRemoveFinalizer))))
			return ctrl.Result{RequeueAfter: r.shortWait}, errors.Wrap(r.kube.Status().Update(ctx, cr), errUpdateResourceStatus)
		}
		return reconcile.Result{Requeue: false}, nil
	}

	if err := r.finalizer.AddFinalizer(ctx, cr); err != nil {
		log.Info(errAddFinalizer, "error", err)
		omitError(log, resource.SetConditions(cr, v1alpha1.ReconcileError(errors.Wrap(err, errAddFinalizer))))
		return ctrl.Result{RequeueAfter: r.shortWait}, errors.Wrap(r.kube.Status().Update(ctx, cr), errUpdateResourceStatus)
	}

	for _, o := range childResources {
		if err := Apply(ctx, r.kube, o); err != nil {
			log.Info("Cannot apply the changes to the child resources", "error", err)
			omitError(log, resource.SetConditions(cr, v1alpha1.ReconcileError(errors.Wrap(err, fmt.Sprintf("%s: %s/%s of type %s", errApply, o.GetName(), o.GetNamespace(), o.GetObjectKind().GroupVersionKind().String())))))
			return ctrl.Result{RequeueAfter: r.shortWait}, errors.Wrap(r.kube.Status().Update(ctx, cr), errUpdateResourceStatus)
		}
	}
	log.Debug("Reconciliation finished with success")
	omitError(log, resource.SetConditions(cr, v1alpha1.ReconcileSuccess()))
	return ctrl.Result{RequeueAfter: r.longWait}, errors.Wrap(r.kube.Status().Update(ctx, cr), errUpdateResourceStatus)
}

// Apply creates if the object doesn't exist and patches if it exists.
func Apply(ctx context.Context, kube client.Client, o resource.ChildResource) error {
	existing := o.DeepCopyObject().(resource.ChildResource)
	err := kube.Get(ctx, types.NamespacedName{Name: o.GetName(), Namespace: o.GetNamespace()}, existing)
	if kerrors.IsNotFound(err) {
		return errors.Wrap(kube.Create(ctx, o), errCreateChildResource)
	}
	if err != nil {
		return errors.Wrap(err, errGetChildResource)
	}
	patchJSON, err := json.Marshal(o)
	if err != nil {
		return err
	}
	return kube.Patch(ctx, existing, client.RawPatch(types.MergePatchType, patchJSON))
}

func omitError(log logging.Logger, err error) {
	if err != nil {
		log.Info("Omitted the non-fatal error", "error", err)
	}
}
