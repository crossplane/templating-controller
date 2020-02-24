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

package fake

import (
	"bytes"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
)

var (
	// MockParentGVK is used as mock GVK of a parent resource.
	MockParentGVK = schema.GroupVersionKind{
		Group:   "mock.parent.crossplane.io",
		Version: "v1alpha1",
		Kind:    "MockResource",
	}
	// MockChildGVK is used as mock GVK of a child resource.
	MockChildGVK = schema.GroupVersionKind{
		Group:   "mock.child.crossplane.io",
		Version: "v1alpha1",
		Kind:    "MockChildResource",
	}
)

// MockResourceOption is used to make manipulations on the *MockResource.
type MockResourceOption func(*MockResource)

// WithGVK returns a MockResourceOption that changes GVK of the *MockResource instance.
func WithGVK(gvk schema.GroupVersionKind) MockResourceOption {
	return func(r *MockResource) {
		r.SetGroupVersionKind(gvk)
	}
}

// WithAdditionalAnnotations returns a MockResourceOption that adds given map as annotations
// to the *MockResource instance.
func WithAdditionalAnnotations(a map[string]string) MockResourceOption {
	return func(r *MockResource) {
		meta.AddAnnotations(r, a)
	}
}

// WithAdditionalLabels returns a MockResourceOption that adds given map as labels
// to the *MockResource instance.
func WithAdditionalLabels(a map[string]string) MockResourceOption {
	return func(r *MockResource) {
		meta.AddLabels(r, a)
	}
}

// WithOwnerReferenceTo returns a MockResourceOption that adds an OwnerReference
// that points to the given object to the *MockResource instance.
func WithOwnerReferenceTo(o metav1.Object, gvk schema.GroupVersionKind) MockResourceOption {
	return func(r *MockResource) {
		ref := meta.AsOwner(meta.ReferenceTo(o, gvk))
		trueVal := true
		ref.BlockOwnerDeletion = &trueVal
		meta.AddOwnerReference(r, ref)
	}
}

// WithNamespaceName returns a MockResourceOption that changes name and namespace
// of the *MockResource instance.
func WithNamespaceName(name, ns string) MockResourceOption {
	return func(r *MockResource) {
		r.SetName(name)
		r.SetNamespace(ns)
	}
}

// FromYAML unmarshals given YAML into the *MockResource instance.
func FromYAML(y []byte) MockResourceOption {
	return func(r *MockResource) {
		dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096)
		err := dec.Decode(&r.Unstructured)
		if err != nil {
			panic(fmt.Sprintf("test yaml is not correct: %s", err.Error()))
		}
	}
}

// NewMockResource returns a new instance of *MockResource.
func NewMockResource(o ...MockResourceOption) *MockResource {
	p := &MockResource{}
	p.SetLabels(map[string]string{})
	p.SetAnnotations(map[string]string{})

	for _, f := range o {
		f(p)
	}

	return p
}

// MockResource is a helper struct to be used in tests.
type MockResource struct {
	unstructured.Unstructured
}
