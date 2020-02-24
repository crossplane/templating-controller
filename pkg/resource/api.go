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

package resource

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane/pkg/stacks"
)

// Constants used for determining whether the defaulting annotations should be
// removed or not.
const (
	RemoveDefaultAnnotationsKey       = "templatestacks.crossplane.io/remove-defaulting-annotations"
	RemoveDefaultAnnotationsTrueValue = "true"
)

// NopTemplatingEngine is a no-op templating engine.
type NopTemplatingEngine struct{}

// Run does nothing.
func (n *NopTemplatingEngine) Run(_ ParentResource) ([]ChildResource, error) {
	return nil, nil
}

// NewOwnerReferenceAdder returns a new *OwnerReferenceAdder
func NewOwnerReferenceAdder() OwnerReferenceAdder {
	return OwnerReferenceAdder{}
}

// OwnerReferenceAdder adds owner reference of ParentResource to all ChildResources
// except the Providers since their deletion should be delayed until all resources
// refer to them are deleted.
type OwnerReferenceAdder struct{}

// Patch patches the child resources with information in ParentResource.
func (lo OwnerReferenceAdder) Patch(cr ParentResource, list []ChildResource) ([]ChildResource, error) {
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

// Patch patches the child resources with information in ParentResource.
func (lo DefaultingAnnotationRemover) Patch(cr ParentResource, list []ChildResource) ([]ChildResource, error) {
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

// Patch patches the child resources with information in ParentResource.
func (lo NamespacePatcher) Patch(cr ParentResource, list []ChildResource) ([]ChildResource, error) {
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

// Patch patches the child resources with information in ParentResource.
func (lo LabelPropagator) Patch(cr ParentResource, list []ChildResource) ([]ChildResource, error) {
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

// Patch patches the child resources with information in ParentResource.
func (lo ParentLabelSetAdder) Patch(cr ParentResource, list []ChildResource) ([]ChildResource, error) {
	for _, o := range list {
		meta.AddLabels(o, stacks.ParentLabels(cr))
	}
	return list, nil
}

// todo: temp solution to detect provider kind.
func isProvider(o runtime.Object) bool {
	gvk := o.GetObjectKind().GroupVersionKind()
	return strings.HasSuffix(gvk.Group, "crossplane.io") && strings.EqualFold(gvk.Kind, "provider")
}
