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
	defaultLongWait  = 3 * time.Minute

	defaultRootPath = "resources"

	errGetResource = "could not get the custom resource"
)

type ConfigurationStackReconcilerOption func(*ConfigurationStackReconciler)

func WithKustomizeOperation(op *operations.KustomizeOperation) ConfigurationStackReconcilerOption {
	return func(reconciler *ConfigurationStackReconciler) {
		reconciler.kustomizeOperation = op
	}
}
func WithPreApplyOverrides(op ...resource.PreApplyOverrider) ConfigurationStackReconcilerOption {
	return func(reconciler *ConfigurationStackReconciler) {
		reconciler.preApplyOverride = resource.PreApplyOverriderChain(op)
	}
}

func NewConfigurationStackReconciler(m manager.Manager, of schema.GroupVersionKind, options ...ConfigurationStackReconcilerOption) *ConfigurationStackReconciler {
	nr := func() resource.ParentResource {
		return runtimeresource.MustCreateObject(schema.GroupVersionKind(of), m.GetScheme()).(resource.ParentResource)
	}
	_ = nr()

	r := &ConfigurationStackReconciler{
		kube:        m.GetClient(),
		newResource: nr,
		shortWait:   defaultShortWait,
		longWait:    defaultLongWait,
		kustomizeOperation: operations.NewKustomizeOperation(defaultRootPath, resource.KustomizeOverriderChain{
			&resource.NamePrefixer{},
			&resource.LabelPropagator{},
		}),
		preApplyOverride: &resource.OwnerReferenceOverrider{},
	}

	for _, opt := range options {
		opt(r)
	}
	return r
}

type ConfigurationStackReconciler struct {
	kube        client.Client
	newResource func() resource.ParentResource
	shortWait   time.Duration
	longWait    time.Duration

	kustomizeOperation *operations.KustomizeOperation
	preApplyOverride   resource.PreApplyOverrider
}

func (r *ConfigurationStackReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
	defer cancel()
	// TODO(muvaf): logging.
	cr := r.newResource()
	if err := r.kube.Get(ctx, req.NamespacedName, cr); err != nil {
		// There's no need to requeue if we no longer exist. Otherwise we'll be
		// requeued implicitly because we return an error.
		return reconcile.Result{}, errors.Wrap(client.IgnoreNotFound(err), errGetResource)
	}

	if meta.WasDeleted(cr) {
		return reconcile.Result{}, nil
	}

	childResources, err := r.kustomizeOperation.RunKustomize(cr)
	if err != nil {
		return ctrl.Result{RequeueAfter: r.shortWait}, errors.Wrap(err, "kustomize operation failed")
	}
	r.preApplyOverride.Override(cr, childResources)
	for _, o := range childResources {
		if err := PatchResource(ctx, r.kube, o); err != nil {
			return ctrl.Result{RequeueAfter: r.shortWait}, errors.Wrap(err, "patch failed")
		}
	}
	cr.SetConditions(v1alpha1.ReconcileSuccess())
	return ctrl.Result{RequeueAfter: r.longWait}, r.kube.Status().Update(ctx, cr)
}

func PatchResource(ctx context.Context, kube client.Client, o resource.ChildResource) error {
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
