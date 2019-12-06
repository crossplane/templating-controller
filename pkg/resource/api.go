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

type PreApplyOverriderFunc func(ParentResource, []ChildResource)

func (pre PreApplyOverriderFunc) Override(cr ParentResource, list []ChildResource) {
	pre(cr, list)
}

type PreApplyOverriderChain []PreApplyOverrider

func (pre PreApplyOverriderChain) Override(cr ParentResource, list []ChildResource) {
	for _, f := range pre {
		f.Override(cr, list)
	}
}

type OwnerReferenceOverrider struct{}

func (lo *OwnerReferenceOverrider) Override(cr ParentResource, list []ChildResource) {
	ref := metav1.OwnerReference{
		APIVersion: cr.GetObjectKind().GroupVersionKind().GroupVersion().String(),
		Kind:       cr.GetObjectKind().GroupVersionKind().Kind,
		Name:       cr.GetName(),
		UID:        cr.GetUID(),
	}
	for _, o := range list {
		if isProvider(o) {
			continue
		}
		meta.AddOwnerReference(o, ref)
	}
}

type KustomizeOverriderFunc func(ParentResource, *types.Kustomization)

func (kof KustomizeOverriderFunc) Process(cr ParentResource, k *types.Kustomization) {
	kof(cr, k)
}

type KustomizeOverriderChain []KustomizeOverrider

func (koc KustomizeOverriderChain) Process(cr ParentResource, k *types.Kustomization) {
	for _, f := range koc {
		f.Process(cr, k)
	}
}

type NamePrefixer struct{}

func (np *NamePrefixer) Process(cr ParentResource, k *types.Kustomization) {
	k.NamePrefix = fmt.Sprintf("%s-", cr.GetName())
}

type LabelPropagator struct{}

func (la *LabelPropagator) Process(cr ParentResource, k *types.Kustomization) {
	if k.CommonLabels == nil {
		k.CommonLabels = map[string]string{}
	}
	k.CommonLabels[fmt.Sprintf("%s/name", cr.GetObjectKind().GroupVersionKind().Group)] = cr.GetName()
	k.CommonLabels[fmt.Sprintf("%s/uid", cr.GetObjectKind().GroupVersionKind().Group)] = string(cr.GetUID())
	for key, val := range cr.GetLabels() {
		k.CommonLabels[key] = val
	}
}

// todo: temp until Provider interface lands on crossplane-runtime.
func isProvider(o runtime.Object) bool {
	return strings.ToLower(o.GetObjectKind().GroupVersionKind().Kind) == "provider"
}
