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
	"github.com/crossplaneio/resourcepacks/pkg/resource"
	"sigs.k8s.io/kustomize/api/types"
)

// A KustomizationPatcher is used to make modifications on Kustomization overlay
// object before the render.
type KustomizationPatcher interface {
	Patch(resource.ParentResource, *types.Kustomization) error
}

// KustomizationPatcherFunc makes it easier to provide only a function as
// KustomizationPatcher
type KustomizationPatcherFunc func(resource.ParentResource, *types.Kustomization) error

func (kof KustomizationPatcherFunc) Patch(cr resource.ParentResource, k *types.Kustomization) error {
	return kof(cr, k)
}

// KustomizationPatcherChain makes it easier to provide a list of KustomizationPatcher
// to be called in order.
type KustomizationPatcherChain []KustomizationPatcher

func (koc KustomizationPatcherChain) Patch(cr resource.ParentResource, k *types.Kustomization) error {
	for _, f := range koc {
		if err := f.Patch(cr, k); err != nil {
			return err
		}
	}
	return nil
}

type OverlayFile struct {
	Name string
	Data []byte
}

type OverlayGenerator interface {
	Generate(resource.ParentResource, *types.Kustomization) ([]OverlayFile, error)
}

// OverlayGeneratorChain makes it easier to provide a list of OverlayGenerator
// to be called in order.
type OverlayGeneratorChain []OverlayGenerator

func (ogc OverlayGeneratorChain) Generate(cr resource.ParentResource, k *types.Kustomization) ([]OverlayFile, error) {
	var result []OverlayFile
	for _, f := range ogc {
		file, err := f.Generate(cr, k)
		if err != nil {
			return nil, err
		}
		result = append(result, file...)
	}
	return result, nil
}
