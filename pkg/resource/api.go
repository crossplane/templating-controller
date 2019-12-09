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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/api/types"

	"github.com/crossplaneio/crossplane-runtime/pkg/meta"
)

func NewOwnerReferenceAdder() *OwnerReferenceAdder {
	return &OwnerReferenceAdder{}
}

type OwnerReferenceAdder struct{}

func (lo *OwnerReferenceAdder) Patch(cr ParentResource, list []ChildResource) ([]ChildResource, error) {
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

func NewNamePrefixer() *NamePrefixer {
	return &NamePrefixer{}
}

type NamePrefixer struct{}

func (np *NamePrefixer) Patch(cr ParentResource, k *types.Kustomization) error {
	k.NamePrefix = fmt.Sprintf("%s-", cr.GetName())
	return nil
}

func NewLabelPropagator() *LabelPropagator {
	return &LabelPropagator{}
}

type LabelPropagator struct{}

func (la *LabelPropagator) Patch(cr ParentResource, k *types.Kustomization) error {
	if k.CommonLabels == nil {
		k.CommonLabels = map[string]string{}
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
