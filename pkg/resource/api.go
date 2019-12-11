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
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/kustomize/api/resid"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/api/types"

	"github.com/crossplaneio/crossplane-runtime/pkg/meta"
)

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

// NewNamePrefixer returns a new *NamePrefixer.
func NewVarReferenceFiller() VariantFiller {
	return VariantFiller{}
}

func isSame(a schema.GroupVersionKind, b resid.Gvk) bool {
	if a.Group != b.Group || a.Version != b.Version || a.Kind != b.Kind {
		return false
	}
	return true
}

// VariantFiller fills the Variants that refer to the ParentResource with the
// correct name and namespace.
type VariantFiller struct{}

func (np VariantFiller) Patch(cr ParentResource, k *types.Kustomization) error {
	if len(k.Vars) == 0 {
		return nil
	}
	newVars := k.Vars
	for _, varRef := range newVars {
		if isSame(cr.GetObjectKind().GroupVersionKind(), varRef.ObjRef.GVK()) {
			varRef.ObjRef.Name = cr.GetName()
			varRef.ObjRef.Namespace = cr.GetNamespace()
		}
	}
	k.Vars = newVars
	return nil
}

// NewNamePrefixer returns a new *NamePrefixer.
func NewNamePrefixer() NamePrefixer {
	return NamePrefixer{}
}

// NamePrefixer adds the name of the ParentResource as name prefix to be used
// in Kustomize.
type NamePrefixer struct{}

func (np NamePrefixer) Patch(cr ParentResource, k *types.Kustomization) error {
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

func (np NamespaceNamePrefixer) Patch(cr ParentResource, k *types.Kustomization) error {
	k.NamePrefix = fmt.Sprintf("%s-%s-", cr.GetNamespace(), cr.GetName())
	return nil
}

// NewLabelPropagator returns a *LabelPropagator
func NewLabelPropagator() LabelPropagator {
	return LabelPropagator{}
}

// LabelPropagator copies all labels of ParentResource to commonLabels of
// Kustomization object so that all rendered resources have those labels.
// It also adds name, namespace(if exists) and uid of the parent resource to the
// commonLabels property.
type LabelPropagator struct{}

func (la LabelPropagator) Patch(cr ParentResource, k *types.Kustomization) error {
	if k.CommonLabels == nil {
		k.CommonLabels = map[string]string{}
	}
	if cr.GetNamespace() != "" {
		k.CommonLabels[fmt.Sprintf("%s/namespace", cr.GetObjectKind().GroupVersionKind().Group)] = cr.GetName()
	}
	k.CommonLabels[fmt.Sprintf("%s/name", cr.GetObjectKind().GroupVersionKind().Group)] = cr.GetName()
	k.CommonLabels[fmt.Sprintf("%s/uid", cr.GetObjectKind().GroupVersionKind().Group)] = string(cr.GetUID())
	for key, val := range cr.GetLabels() {
		k.CommonLabels[key] = val
	}
	return nil
}

// todo: temp until Provider interface lands on crossplane-runtime.
func isProvider(o runtime.Object) bool {
	return strings.ToLower(o.GetObjectKind().GroupVersionKind().Kind) == "provider"
}
