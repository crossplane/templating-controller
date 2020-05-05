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
	"strconv"
	"strings"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane/pkg/stacks"

	"github.com/crossplane/templating-controller/pkg/resource"
)

// Error strings.
const (
	errDeleteChildResource = "cannot delete child resource"
	errPriorityToInt       = "cannot convert deletion priority into integer"
)

// Constants used for annotations.
const (
	RemoveDefaultAnnotationsKey         = "templatestacks.crossplane.io/remove-defaulting-annotations"
	RemoveDefaultAnnotationsTrueValue   = "true"
	DeletionPriorityAnnotationKey       = "templatestacks.crossplane.io/deletion-priority"
	DeletionPriorityAnnotationZeroValue = "0"
)

const minInt = -(int(^uint(0) >> 1)) - 1

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
	ref := meta.AsOwner(meta.ReferenceTo(cr, cr.GroupVersionKind()))
	trueVal := true
	ref.BlockOwnerDeletion = &trueVal
	for _, o := range list {
		// TODO(muvaf): Provider kind resources are special in the sense that
		// their deletion should be blocked until all resources provisioned with
		// them are deleted. Since we let Kubernetes garbage collector clean the
		// resources, we skip deletion of Provider kind resources for deletions
		// to success.
		// Find a way to realize that dependency without bringing in too much
		// complexity.
		if isProvider(o) {
			continue
		}
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
		meta.AddLabels(o, stacks.ParentLabels(cr))
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
func (d *APIOrderedDeleter) Delete(ctx context.Context, list []resource.ChildResource) ([]resource.ChildResource, error) {
	hp := minInt
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
		p, err := strconv.Atoi(val)
		if err != nil {
			return nil, errors.Wrap(err, errPriorityToInt)
		}
		// We directly skip the case where we are sure that the resource
		// has a lower priority than current highest priority regardless of
		// whether it exists or not.
		if p < hp {
			continue
		}
		nn := types.NamespacedName{Name: res.GetName(), Namespace: res.GetNamespace()}
		if err := d.kube.Get(ctx, nn, res); err != nil {
			// The resources that do not exist anymore should not have any
			// effect in our calculations.
			if kerrors.IsNotFound(err) {
				continue
			}
			return nil, errors.Wrap(err, errGetChildResource)
		}
		// If the priority of the resource is higher than our current highest
		// priority level, then we should discard everything in the deletion queue
		// and set the new highest priority.
		if p > hp {
			hp = p
			del = []resource.ChildResource{res}
			continue
		}
		// If the resource is on the highest priority level, then it should be
		// deleted in this iteration.
		del = append(del, res)
	}
	for _, res := range del {
		if err := d.kube.Delete(ctx, res); client.IgnoreNotFound(err) != nil {
			return nil, errors.Wrap(err, errDeleteChildResource)
		}
	}
	return del, nil
}

// todo: temp solution to detect provider kind.
func isProvider(o runtime.Object) bool {
	gvk := o.GetObjectKind().GroupVersionKind()
	return strings.HasSuffix(gvk.Group, "crossplane.io") && strings.EqualFold(gvk.Kind, "provider")
}
