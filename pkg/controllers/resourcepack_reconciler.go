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
package controllers

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/net/context"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplaneio/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplaneio/crossplane-runtime/pkg/meta"
	runtimeresource "github.com/crossplaneio/crossplane-runtime/pkg/resource"

	"github.com/muvaf/configuration-stacks/pkg/operations"
	"github.com/muvaf/configuration-stacks/pkg/resource"
)

const (
	reconcileTimeout = 1 * time.Minute

	defaultShortWait = 30 * time.Second
	defaultLongWait  = 1 * time.Minute

	defaultRootPath = "resources"

	errUpdateResourceStatus  = "could not update status of the custom resource"
	errGetResource           = "could not get the custom resource"
	errKustomizeOperation    = "kustomize operation failed"
	errChildResourcePatchers = "child resource patchers failed"
	errApply                 = "apply failed"
)

type ResurcePackReconcilerOption func(*ResurcePackReconciler)

func AdditionalKustomizationPatcher(op ...resource.KustomizationPatcher) ResurcePackReconcilerOption {
	return func(reconciler *ResurcePackReconciler) {
		reconciler.kustomizeOperation.Patcher = append(reconciler.kustomizeOperation.Patcher, op...)
	}
}
func AdditionalChildResourcePatcher(op ...resource.ChildResourcePatcher) ResurcePackReconcilerOption {
	return func(reconciler *ResurcePackReconciler) {
		reconciler.childResourcePatcher = append(reconciler.childResourcePatcher, op...)
	}
}

func WithResourcePath(path string) ResurcePackReconcilerOption {
	return func(reconciler *ResurcePackReconciler) {
		reconciler.kustomizeOperation.ResourcePath = path
	}
}

func WithShortWait(d time.Duration) ResurcePackReconcilerOption {
	return func(reconciler *ResurcePackReconciler) {
		reconciler.shortWait = d
	}
}

func WithLongWait(d time.Duration) ResurcePackReconcilerOption {
	return func(reconciler *ResurcePackReconciler) {
		reconciler.longWait = d
	}
}

func NewResurcePackReconciler(m manager.Manager, of schema.GroupVersionKind, options ...ResurcePackReconcilerOption) *ResurcePackReconciler {
	nr := func() resource.ParentResource {
		return runtimeresource.MustCreateObject(schema.GroupVersionKind(of), m.GetScheme()).(resource.ParentResource)
	}
	// Early panic if the resource doesn't satisfy ParentResource interface.
	_ = nr()

	r := &ResurcePackReconciler{
		kube:              m.GetClient(),
		newParentResource: nr,
		shortWait:         defaultShortWait,
		longWait:          defaultLongWait,
		kustomizeOperation: operations.NewKustomizeOperation(defaultRootPath, resource.KustomizationPatcherChain{
			resource.NewNamePrefixer(),
			resource.NewLabelPropagator(),
			resource.NewVarReferenceFiller(),
		}),
		childResourcePatcher: resource.ChildResourcePatcherChain{
			resource.NewDefaultingAnnotationRemover(),
			resource.NewOwnerReferenceAdder(),
		},
	}

	for _, opt := range options {
		opt(r)
	}
	return r
}

type ResurcePackReconciler struct {
	kube              client.Client
	newParentResource func() resource.ParentResource
	resourcePath      string
	shortWait         time.Duration
	longWait          time.Duration

	kustomizeOperation   *operations.KustomizeOperation
	childResourcePatcher resource.ChildResourcePatcherChain
}

func (r *ResurcePackReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
	defer cancel()

	cr := r.newParentResource()
	if err := r.kube.Get(ctx, req.NamespacedName, cr); err != nil {
		// There's no need to requeue if the resource no longer exists. Otherwise
		// we'll be requeued implicitly because we return an error.
		return reconcile.Result{Requeue: false}, errors.Wrap(client.IgnoreNotFound(err), errGetResource)
	}

	if meta.WasDeleted(cr) {
		// We have nothing to do as the child resources will be garbage collected
		// by Kubernetes.
		return reconcile.Result{Requeue: false}, nil
	}

	childResources, err := r.kustomizeOperation.Run(cr)
	if err != nil {
		cr.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, errKustomizeOperation)))
		return ctrl.Result{RequeueAfter: r.shortWait}, errors.Wrap(r.kube.Status().Update(ctx, cr), errUpdateResourceStatus)
	}

	childResources, err = r.childResourcePatcher.Patch(cr, childResources)
	if err != nil {
		cr.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, errChildResourcePatchers)))
		return ctrl.Result{RequeueAfter: r.shortWait}, errors.Wrap(r.kube.Status().Update(ctx, cr), errUpdateResourceStatus)
	}

	for _, o := range childResources {
		if err := Apply(ctx, r.kube, o); err != nil {
			cr.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, fmt.Sprintf("%s: %s/%s of type %s", errApply, o.GetName(), o.GetNamespace(), o.GetObjectKind().GroupVersionKind().String()))))
			return ctrl.Result{RequeueAfter: r.shortWait}, errors.Wrap(r.kube.Status().Update(ctx, cr), errUpdateResourceStatus)
		}
	}

	cr.SetConditions(v1alpha1.ReconcileSuccess())
	return ctrl.Result{RequeueAfter: r.longWait}, errors.Wrap(r.kube.Status().Update(ctx, cr), errUpdateResourceStatus)
}

// Apply creates if the object doesn't exist and patches if it does exists.
func Apply(ctx context.Context, kube client.Client, o resource.ChildResource) error {
	existing := o.DeepCopyObject().(resource.ChildResource)
	err := kube.Get(ctx, types.NamespacedName{Name: o.GetName(), Namespace: o.GetNamespace()}, existing)
	if kerrors.IsNotFound(err) {
		return kube.Create(ctx, o)
	}
	if err != nil {
		return err
	}
	o.SetResourceVersion(existing.GetResourceVersion())
	return kube.Patch(ctx, o, client.MergeFrom(existing))
}
