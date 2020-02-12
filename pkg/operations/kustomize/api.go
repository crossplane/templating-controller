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
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/kustomize/api/resid"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/yaml"

	"github.com/crossplaneio/crossplane/apis/stacks/v1alpha1"

	"github.com/crossplaneio/templating-controller/pkg/resource"
)

// NewNamePrefixer returns a new *NamePrefixer.
func NewVarReferenceFiller() VariantFiller {
	return VariantFiller{}
}

func getSchemaGVK(gvk resid.Gvk) schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   gvk.Group,
		Version: gvk.Version,
		Kind:    gvk.Kind,
	}
}

// VariantFiller fills the Variants that refer to the ParentResource with the
// correct name and namespace.
type VariantFiller struct{}

func (np VariantFiller) Patch(cr resource.ParentResource, k *types.Kustomization) error {
	if len(k.Vars) == 0 {
		return nil
	}
	for i, varRef := range k.Vars {
		if cr.GetObjectKind().GroupVersionKind() == getSchemaGVK(varRef.ObjRef.GVK()) {
			k.Vars[i].ObjRef.Name = cr.GetName()
			k.Vars[i].ObjRef.Namespace = cr.GetNamespace()
		}
	}
	return nil
}

// NewNamePrefixer returns a new *NamePrefixer.
func NewNamePrefixer() NamePrefixer {
	return NamePrefixer{}
}

// NamePrefixer adds the name of the ParentResource as name prefix to be used
// in Kustomize.
type NamePrefixer struct{}

func (np NamePrefixer) Patch(cr resource.ParentResource, k *types.Kustomization) error {
	k.NamePrefix = fmt.Sprintf("%s-", cr.GetName())
	return nil
}

// NewNamePrefixer returns a new *NamePrefixer.
func NewNamespaceNamePrefixer() NamespaceNamePrefixer {
	return NamespaceNamePrefixer{}
}

// NamePrefixer adds the name of the ParentResource as name prefix to be used
// in Kustomize.
type NamespaceNamePrefixer struct{}

func (np NamespaceNamePrefixer) Patch(cr resource.ParentResource, k *types.Kustomization) error {
	k.NamePrefix = fmt.Sprintf("%s-%s-", cr.GetNamespace(), cr.GetName())
	return nil
}

// NewPatchOverlayGenerator returns a new PatchOverlayGenerator.
func NewPatchOverlayGenerator(overlays []v1alpha1.KustomizeEngineOverlay) PatchOverlayGenerator {
	return PatchOverlayGenerator{
		Overlays: overlays,
	}
}

// NamePrefixer adds the name of the ParentResource as name prefix to be used
// in Kustomize.
type PatchOverlayGenerator struct {
	Overlays []v1alpha1.KustomizeEngineOverlay
}

func (pog PatchOverlayGenerator) Generate(cr resource.ParentResource, k *types.Kustomization) ([]OverlayFile, error) {
	if len(pog.Overlays) == 0 {
		return nil, nil
	}
	finalOverlayYAML := ""
	for _, overlay := range pog.Overlays {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(overlay.APIVersion)
		obj.SetKind(overlay.Kind)
		obj.SetName(overlay.Name)
		// todo: stackdefinition does not support namespace yet.
		// obj.SetNamespace(overlay.Namespace)

		for _, binding := range overlay.Bindings {
			// First make sure there is a value in the referred path.
			val, exists, err := unstructured.NestedFieldCopy(cr.UnstructuredContent(), strings.Split(binding.From, ".")...)
			if err != nil {
				return nil, err
			}
			if !exists {
				continue
			}
			if err := unstructured.SetNestedField(obj.Object, val, strings.Split(binding.To, ".")...); err != nil {
				return nil, err
			}
		}
		overlayYAML, err := yaml.Marshal(obj)
		if err != nil {
			return nil, err
		}
		// TODO(muvaf): yaml.Marshal does not support outputting multiple YAML
		// documents. That's temporary solution.
		finalOverlayYAML = fmt.Sprintf("%s---\n%s", finalOverlayYAML, string(overlayYAML))
	}
	fileName := "overlaypatch.yaml"
	k.PatchesStrategicMerge = appendPatchMergeIfNotExists(k.PatchesStrategicMerge, types.PatchStrategicMerge(fileName))
	return []OverlayFile{
		{
			Name: fileName,
			Data: []byte(finalOverlayYAML),
		},
	}, nil
}

// todo: temporary.
func appendPatchMergeIfNotExists(arr []types.PatchStrategicMerge, obj types.PatchStrategicMerge) []types.PatchStrategicMerge {
	for _, e := range arr {
		if e == obj {
			return arr
		}
	}
	return append(arr, obj)
}
