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
	"github.com/crossplaneio/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplaneio/templating-controller/pkg/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/json"
)

var (
	_ resource.ParentResource = &MockParentResource{}
	_ resource.ChildResource  = &MockChildResource{}
)

type MockParentResource struct {
	metav1.ObjectMeta
	v1alpha1.ConditionedStatus
}

func (m *MockParentResource) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

func (m *MockParentResource) DeepCopyObject() runtime.Object {
	out := &MockParentResource{}
	j, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	_ = json.Unmarshal(j, out)
	return out
}

func (m *MockParentResource) UnstructuredContent() map[string]interface{} {
	return nil
}

type MockChildResource struct {
	metav1.ObjectMeta
}

func (m *MockChildResource) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

func (m *MockChildResource) DeepCopyObject() runtime.Object {
	out := &MockChildResource{}
	j, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	_ = json.Unmarshal(j, out)
	return out
}
