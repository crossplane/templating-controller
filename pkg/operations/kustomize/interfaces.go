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

package kustomize

import (
	"github.com/crossplane/templating-controller/pkg/resource"
	"sigs.k8s.io/kustomize/api/types"
)

// Option is used to manipulate default Engine parameters.
type Option func(*Engine)

// A Patcher is used to make modifications on Kustomization overlay
// object before the render.
type Patcher interface {
	Patch(resource.ParentResource, *types.Kustomization) error
}

// PatcherFunc makes it easier to provide only a function as
// Patcher
type PatcherFunc func(resource.ParentResource, *types.Kustomization) error

// Patch patches the given *types.Kustomization with information in resource.ParentResource.
func (kof PatcherFunc) Patch(cr resource.ParentResource, k *types.Kustomization) error {
	return kof(cr, k)
}

// KustomizationPatcherChain makes it easier to provide a list of Patcher
// to be called in order.
type KustomizationPatcherChain []Patcher

// Patch patches the given *types.Kustomization with information in resource.ParentResource.
func (koc KustomizationPatcherChain) Patch(cr resource.ParentResource, k *types.Kustomization) error {
	for _, f := range koc {
		if err := f.Patch(cr, k); err != nil {
			return err
		}
	}
	return nil
}

// OverlayFile is used to represent the files to be written to the top overlay
// folder used during kustomization operation.
type OverlayFile struct {
	Name string
	Data []byte
}

// OverlayGenerator is used for generating files to be written to the top kustomization
// folder.
type OverlayGenerator interface {
	Generate(resource.ParentResource, *types.Kustomization) ([]OverlayFile, error)
}

// OverlayGeneratorChain makes it easier to provide a list of OverlayGenerator
// to be called in order.
type OverlayGeneratorChain []OverlayGenerator

// Generate generates OverlayFiles.
func (ogc OverlayGeneratorChain) Generate(cr resource.ParentResource, k *types.Kustomization) ([]OverlayFile, error) {
	result := []OverlayFile{}
	for _, f := range ogc {
		file, err := f.Generate(cr, k)
		if err != nil {
			return nil, err
		}
		result = append(result, file...)
	}
	return result, nil
}
