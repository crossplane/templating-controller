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
	MockParentGVK = schema.GroupVersionKind{
		Group:   "mock.parent.crossplane.io",
		Version: "v1alpha1",
		Kind:    "MockResource",
	}
	MockChildGVK = schema.GroupVersionKind{
		Group:   "mock.child.crossplane.io",
		Version: "v1alpha1",
		Kind:    "MockChildResource",
	}
)

type MockResourceOption func(*MockResource)

func WithGVK(gvk schema.GroupVersionKind) MockResourceOption {
	return func(r *MockResource) {
		r.SetGroupVersionKind(gvk)
	}
}

func WithAnnotations(a map[string]string) MockResourceOption {
	return func(r *MockResource) {
		meta.AddAnnotations(r, a)
	}
}

func WithLabels(a map[string]string) MockResourceOption {
	return func(r *MockResource) {
		meta.AddLabels(r, a)
	}
}

func WithOwnerReferenceTo(o metav1.Object, gvk schema.GroupVersionKind) MockResourceOption {
	return func(r *MockResource) {
		ref := meta.ReferenceTo(o, gvk)
		meta.AddOwnerReference(r, meta.AsOwner(ref))
	}
}

func WithNamespaceName(name, ns string) MockResourceOption {
	return func(r *MockResource) {
		r.SetName(name)
		r.SetNamespace(ns)
	}
}

func FromYAML(y []byte) MockResourceOption {
	return func(r *MockResource) {
		dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(y), 4096)
		err := dec.Decode(&r.Unstructured)
		if err != nil {
			panic(fmt.Sprintf("test yaml is not correct: %s", err.Error()))
		}
	}
}

func NewMockResource(o ...MockResourceOption) *MockResource {
	p := &MockResource{}
	p.SetLabels(map[string]string{})
	p.SetAnnotations(map[string]string{})

	for _, f := range o {
		f(p)
	}

	return p
}

type MockResource struct {
	unstructured.Unstructured
}
