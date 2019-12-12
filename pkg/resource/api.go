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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/crossplaneio/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplaneio/crossplane-runtime/pkg/meta"
)

const KeepDefaultAnnotationsKey = "resourcepacks.crossplane.io/keep-defaulting-annotations"
const KeepDefaultAnnotationsTrueValue = "true"

// NewOwnerReferenceAdder returns a new *OwnerReferenceAdder
func NewOwnerReferenceAdder() OwnerReferenceAdder {
	return OwnerReferenceAdder{}
}

// OwnerReferenceAdder adds owner reference of ParentResource to all ChildResources
// except the Providers since their deletion should be delayed until all resources
// refer to them are deleted.
type OwnerReferenceAdder struct{}

func (lo OwnerReferenceAdder) Patch(cr ParentResource, list []ChildResource) ([]ChildResource, error) {
	ref := metav1.OwnerReference{
		APIVersion: cr.GetObjectKind().GroupVersionKind().GroupVersion().String(),
		Kind:       cr.GetObjectKind().GroupVersionKind().Kind,
		Name:       cr.GetName(),
		UID:        cr.GetUID(),
	}
	for _, o := range list {
		// TODO(muvaf): Provider kind resources are special in the sense that
		// their deletion should be blocked until all resources provisioned with
		// them are deleted. Since we let Kubernets garbage collector clean the
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
// if not explicitly specified otherwise.
type DefaultingAnnotationRemover struct{}

func (lo DefaultingAnnotationRemover) Patch(cr ParentResource, list []ChildResource) ([]ChildResource, error) {
	if cr.GetAnnotations()[KeepDefaultAnnotationsKey] == KeepDefaultAnnotationsTrueValue {
		return list, nil
	}
	for _, o := range list {
		meta.RemoveAnnotations(o, v1alpha1.AnnotationDefaultClassKey)
	}
	return list, nil
}

// todo: temp until Provider interface lands on crossplane-runtime.
func isProvider(o runtime.Object) bool {
	return strings.ToLower(o.GetObjectKind().GroupVersionKind().Kind) == "provider"
}
