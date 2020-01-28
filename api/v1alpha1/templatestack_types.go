/*
Copyright 2020 The Crossplane Authors.

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

// TODO: This CRD is fake until actual CRD lands on Crossplane repo.

package v1alpha1

import (
	"github.com/crossplaneio/crossplane/apis/stacks/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TemplateStackSpec defines the desired state of TemplateStack
type TemplateStackSpec struct {
	v1alpha1.AppMetadataSpec `json:",inline"`
	Permissions              v1alpha1.PermissionsSpec `json:"permissions,omitempty"`
	Behavior                 Behavior                 `json:"behavior,omitempty"`
}
type Behavior struct {
	CRD                 v1.ObjectReference  `json:"crd,omitempty"`
	EngineConfiguration EngineConfiguration `json:"engineConfiguration,omitempty"`
	Source              Source              `json:"source,omitempty"`
}

type Source struct {
	Image string `json:"image,omitempty"`

	// Path in the image or whatever filesystem you pulled in.
	Path string `json:"path,omitempty"`
}

type EngineConfiguration struct {
	Type                    string `json:"type,omitempty"`
	*KustomizeConfiguration `json:",inline"`
}

type KustomizeConfiguration struct {
	Overlays []Overlay `json:"overlays,omitempty"`
	// types.Kustomization type has no deepcopy methods. We need to cast this into
	// types.Kustomization object.
	Kustomization *unstructured.Unstructured `json:"kustomization,omitempty"`
}

type Overlay struct {
	v1.ObjectReference `json:",inline"`
	Bindings           []FieldBinding `json:"bindings"`
}

type FieldBinding struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// TemplateStackStatus defines the observed state of TemplateStack
type TemplateStackStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true

// TemplateStack is the Schema for the templatestacks API
type TemplateStack struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TemplateStackSpec   `json:"spec,omitempty"`
	Status TemplateStackStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TemplateStackList contains a list of TemplateStack
type TemplateStackList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TemplateStack `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TemplateStack{}, &TemplateStackList{})
}
