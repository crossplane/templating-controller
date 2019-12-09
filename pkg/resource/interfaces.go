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
	"sigs.k8s.io/kustomize/api/types"

	"github.com/crossplaneio/crossplane-runtime/pkg/resource"
)

type ParentResource interface {
	runtime.Object
	metav1.Object

	resource.Conditioned
}

type ChildResource interface {
	runtime.Object
	metav1.Object
}

type KustomizationPatcher interface {
	Patch(ParentResource, *types.Kustomization) error
}

type ChildResourcePatcher interface {
	Patch(ParentResource, []ChildResource) ([]ChildResource, error)
}

type KustomizationPatcherFunc func(ParentResource, *types.Kustomization) error

func (kof KustomizationPatcherFunc) Patch(cr ParentResource, k *types.Kustomization) error {
	return kof(cr, k)
}

type KustomizationPatcherChain []KustomizationPatcher

func (koc KustomizationPatcherChain) Patch(cr ParentResource, k *types.Kustomization) error {
	for _, f := range koc {
		if err := f.Patch(cr, k); err != nil {
			return err
		}
	}
	return nil
}

type ChildResourcePatcherFunc func(ParentResource, []ChildResource) ([]ChildResource, error)

func (pre ChildResourcePatcherFunc) Patch(cr ParentResource, list []ChildResource) ([]ChildResource, error) {
	return pre(cr, list)
}

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
