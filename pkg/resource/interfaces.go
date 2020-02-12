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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ParentResource should be satisfied by the stack CRD that would like to use
// Templating Reconciler.
type ParentResource interface {
	runtime.Object
	metav1.Object

	UnstructuredContent() map[string]interface{}
	GroupVersionKind() schema.GroupVersionKind
}

// ChildResource is satisfied by all Kubernetes objects that the stack may want
// to render and deploy.
type ChildResource interface {
	runtime.Object
	metav1.Object
}

type TemplatingEngine interface {
	Run(ParentResource) ([]ChildResource, error)
}

// ChildResourcePatcher operates on the resources rendered by the templating
// engine.
type ChildResourcePatcher interface {
	Patch(ParentResource, []ChildResource) ([]ChildResource, error)
}

// ChildResourcePatcherFunc makes it easier to provide only a function as
// ChildResourcePatcher
type ChildResourcePatcherFunc func(ParentResource, []ChildResource) ([]ChildResource, error)

func (pre ChildResourcePatcherFunc) Patch(cr ParentResource, list []ChildResource) ([]ChildResource, error) {
	return pre(cr, list)
}

// ChildResourcePatcherChain makes it easier to provide a list of ChildResourcePatcher
// to be called in order.
type ChildResourcePatcherChain []ChildResourcePatcher

func (pre ChildResourcePatcherChain) Patch(cr ParentResource, list []ChildResource) ([]ChildResource, error) {
	currentList := list
	var err error
	for _, f := range pre {
		currentList, err = f.Patch(cr, currentList)
		if err != nil {
			return nil, err
		}
	}
	return currentList, nil
}
