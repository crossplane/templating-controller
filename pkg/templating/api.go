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
	"context"
	"math"
	"strconv"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	rresource "github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane/pkg/packages"

	"github.com/crossplane/templating-controller/pkg/resource"
)

// Error strings.
const (
	errDeleteChildResource = "cannot delete child resource"
	errPriorityToInt       = "cannot convert deletion priority into integer"
	errNotController       = "child resource is not controlled by given parent"
)

// Constants used for annotations.
const (
	RemoveDefaultAnnotationsKey         = "templatestacks.crossplane.io/remove-defaulting-annotations"
	RemoveDefaultAnnotationsTrueValue   = "true"
	DeletionPriorityAnnotationKey       = "templatestacks.crossplane.io/deletion-priority"
	DeletionPriorityAnnotationZeroValue = "0"
)

// NopEngine is a no-op templating engine.
type NopEngine struct{}

// Run does nothing.
func (n *NopEngine) Run(_ resource.ParentResource) ([]resource.ChildResource, error) {
	return nil, nil
}

// NewOwnerReferenceAdder returns a new *OwnerReferenceAdder
func NewOwnerReferenceAdder() OwnerReferenceAdder {
	return OwnerReferenceAdder{}
}

// OwnerReferenceAdder adds owner reference of resource.ParentResource to all resource.ChildResources
// except the Providers since their deletion should be delayed until all resources
// refer to them are deleted.
type OwnerReferenceAdder struct{}

// Patch patches the child resources with information in resource.ParentResource.
func (lo OwnerReferenceAdder) Patch(cr resource.ParentResource, list []resource.ChildResource) ([]resource.ChildResource, error) {
	ref := meta.AsController(meta.ReferenceTo(cr, cr.GroupVersionKind()))
	trueVal := true
	ref.BlockOwnerDeletion = &trueVal
	for _, o := range list {
		meta.AddOwnerReference(o, ref)
	}
	return list, nil
}

// NewDefaultingAnnotationRemover returns a new DefaultingAnnotationRemover
func NewDefaultingAnnotationRemover() DefaultingAnnotationRemover {
	return DefaultingAnnotationRemover{}
}

// DefaultingAnnotationRemover removes the defaulting annotation on the resources
// if it is requested through the special annotation.
type DefaultingAnnotationRemover struct{}

// Patch patches the child resources with information in resource.ParentResource.
func (lo DefaultingAnnotationRemover) Patch(cr resource.ParentResource, list []resource.ChildResource) ([]resource.ChildResource, error) {
	if cr.GetAnnotations()[RemoveDefaultAnnotationsKey] != RemoveDefaultAnnotationsTrueValue {
		return list, nil
	}
	for _, o := range list {
		meta.RemoveAnnotations(o, v1alpha1.AnnotationDefaultClassKey)
	}
	return list, nil
}

// NewNamespacePatcher returns a new NamespacePatcher
func NewNamespacePatcher() NamespacePatcher {
	return NamespacePatcher{}
}

// NamespacePatcher patches the child resources whose metadata.namespace is empty
// with namespace of the parent resource. Note that we don't need to know whether
// child resource is cluster-scoped or not because even though it is, the creation
// goes through with no error, namespace being skipped.
type NamespacePatcher struct{}

// Patch patches the child resources with information in resource.ParentResource.
func (lo NamespacePatcher) Patch(cr resource.ParentResource, list []resource.ChildResource) ([]resource.ChildResource, error) {
	if cr.GetNamespace() == "" {
		return list, nil
	}
	for _, o := range list {
		if o.GetNamespace() == "" {
			o.SetNamespace(cr.GetNamespace())
		}
	}
	return list, nil
}

// NewLabelPropagator returns a new LabelPropagator
func NewLabelPropagator() LabelPropagator {
	return LabelPropagator{}
}

// LabelPropagator propagates all the labels that the parent resource has down
// to all child resources.
type LabelPropagator struct{}

// Patch patches the child resources with information in resource.ParentResource.
func (lo LabelPropagator) Patch(cr resource.ParentResource, list []resource.ChildResource) ([]resource.ChildResource, error) {
	for _, o := range list {
		meta.AddLabels(o, cr.GetLabels())
	}
	return list, nil
}

// NewParentLabelSetAdder returns a new ParentLabelSetAdder
func NewParentLabelSetAdder() ParentLabelSetAdder {
	return ParentLabelSetAdder{}
}

// ParentLabelSetAdder adds parent labels to the child resources.
// See https://github.com/crossplane/crossplane/blob/master/design/one-pager-stack-relationship-labels.md
type ParentLabelSetAdder struct{}

// Patch patches the child resources with information in resource.ParentResource.
func (lo ParentLabelSetAdder) Patch(cr resource.ParentResource, list []resource.ChildResource) ([]resource.ChildResource, error) {
	for _, o := range list {
		meta.AddLabels(o, packages.ParentLabels(cr))
	}
	return list, nil
}

// NewAPIOrderedDeleter returns a new *APIOrderedDeleter.
func NewAPIOrderedDeleter(c client.Client) *APIOrderedDeleter {
	return &APIOrderedDeleter{kube: c}
}

// APIOrderedDeleter deletes the child resources in an order that is determined
// by their priority noted in the child resource annotation. The child resources
// with higher priority will be deleted first and their deletion will block
// the lower priority ones.
type APIOrderedDeleter struct {
	kube client.Client
}

// Delete executes an ordered deletion of child resources depending on their
// deletion priority.
func (d *APIOrderedDeleter) Delete(ctx context.Context, cr resource.ParentResource, list []resource.ChildResource) ([]resource.ChildResource, error) {
	hp := int64(math.MinInt64)
	del := []resource.ChildResource{}
	for _, res := range list {
		val, ok := res.GetAnnotations()[DeletionPriorityAnnotationKey]
		// The zero-value sets a default but it doesn't necessarily mean that the
		// resources with no annotation will be deleted last as user may want to
		// mark some resources as last-to-be-deleted by giving them negative
		// priority.
		if !ok {
			val = DeletionPriorityAnnotationZeroValue
		}
		p, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return nil, errors.Wrap(err, errPriorityToInt)
		}

		nn := types.NamespacedName{Name: res.GetName(), Namespace: res.GetNamespace()}
		err = d.kube.Get(ctx, nn, res)
		if client.IgnoreNotFound(err) != nil {
			return nil, errors.Wrap(err, errGetChildResource)
		}
		// The resources that do not exist anymore should not have any
		// effect in our calculations.
		if kerrors.IsNotFound(err) {
			continue
		}
		// A new high should reset the deletion list and set the new highest.
		// If the resource is on the same priority level, then it should be added
		// to the deletion list. If it's neither same or higher, then it should
		// be skipped.
		switch {
		case p > hp:
			hp = p
			del = []resource.ChildResource{res}
		case p == hp:
			del = append(del, res)
		}
	}
	for _, res := range del {
		if err := d.deleteIfControllable(ctx, res, cr); err != nil {
			return nil, err
		}
	}
	return del, nil
}

// TODO(muvaf): This function is similar to Apply with MustBeControllableBy option
// and should be in crossplane-runtime.
func (d *APIOrderedDeleter) deleteIfControllable(ctx context.Context, obj, controller rresource.Object) error {
	if metav1.GetControllerOf(obj) != nil && !metav1.IsControlledBy(obj, controller) {
		return errors.New(errNotController)
	}
	return errors.Wrap(client.IgnoreNotFound(d.kube.Delete(ctx, obj)), errDeleteChildResource)
}
