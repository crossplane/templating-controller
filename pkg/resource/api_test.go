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
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/crossplane/crossplane/pkg/stacks"

	"github.com/crossplane/templating-controller/pkg/resource/fake"
)

const (
	name      = "fakename"
	namespace = "fakenamespace"
	uid       = types.UID("9e23-sda231-sad")
)

var (
	_ ChildResourcePatcher = OwnerReferenceAdder{}
	_ ChildResourcePatcher = DefaultingAnnotationRemover{}
	_ ChildResourcePatcher = NamespacePatcher{}
	_ ChildResourcePatcher = LabelPropagator{}
	_ ChildResourcePatcher = ParentLabelSetAdder{}
)

type args struct {
	cr   ParentResource
	list []ChildResource
}

type want struct {
	result []ChildResource
	err    error
	reason string
}

func TestDefaultingAnnotationRemover(t *testing.T) {
	cases := map[string]struct {
		args
		want
	}{
		"KeepAnnotated": {
			args: args{
				cr: fake.NewMockResource(),
				list: []ChildResource{
					fake.NewMockResource(fake.WithAnnotations(map[string]string{v1alpha1.AnnotationDefaultClassKey: v1alpha1.AnnotationDefaultClassValue})),
					fake.NewMockResource(fake.WithAnnotations(map[string]string{v1alpha1.AnnotationDefaultClassKey: v1alpha1.AnnotationDefaultClassValue})),
				},
			},
			want: want{
				result: []ChildResource{
					fake.NewMockResource(fake.WithAnnotations(map[string]string{v1alpha1.AnnotationDefaultClassKey: v1alpha1.AnnotationDefaultClassValue})),
					fake.NewMockResource(fake.WithAnnotations(map[string]string{v1alpha1.AnnotationDefaultClassKey: v1alpha1.AnnotationDefaultClassValue})),
				},
			},
		},
		"RemoveAnnotation": {
			args: args{
				cr: fake.NewMockResource(fake.WithAnnotations(map[string]string{RemoveDefaultAnnotationsKey: RemoveDefaultAnnotationsTrueValue})),
				list: []ChildResource{
					fake.NewMockResource(fake.WithAnnotations(map[string]string{v1alpha1.AnnotationDefaultClassKey: v1alpha1.AnnotationDefaultClassValue})),
					fake.NewMockResource(),
				},
			},
			want: want{
				result: []ChildResource{
					fake.NewMockResource(),
					fake.NewMockResource(),
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := NewDefaultingAnnotationRemover()
			got, err := p.Patch(tc.args.cr, tc.args.list)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}
		})
	}
}

func TestOwnerReferenceAdder(t *testing.T) {
	parent := fake.NewMockResource(func(r *fake.MockResource) {
		r.SetName(name)
		r.SetNamespace(namespace)
		r.SetUID(name)
	})
	cases := map[string]struct {
		args
		want
	}{
		"Add": {
			args: args{
				cr: parent,
				list: []ChildResource{
					fake.NewMockResource(),
					fake.NewMockResource(),
					fake.NewMockResource(fake.WithGVK(schema.GroupVersionKind{
						Group:   "gcp.crossplane.io",
						Version: "v1alpha1",
						Kind:    "provider",
					})),
				},
			},
			want: want{
				result: []ChildResource{
					fake.NewMockResource(fake.WithOwnerReferenceTo(parent, parent.GroupVersionKind())),
					fake.NewMockResource(fake.WithOwnerReferenceTo(parent, parent.GroupVersionKind())),
					fake.NewMockResource(fake.WithGVK(schema.GroupVersionKind{
						Group:   "gcp.crossplane.io",
						Version: "v1alpha1",
						Kind:    "provider",
					})),
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := NewOwnerReferenceAdder()
			got, err := p.Patch(tc.args.cr, tc.args.list)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}
		})
	}
}

func TestNamespacePatcher(t *testing.T) {
	cases := map[string]struct {
		args
		want
	}{
		"Patch": {
			args: args{
				cr: fake.NewMockResource(fake.WithNamespaceName("", namespace)),
				list: []ChildResource{
					fake.NewMockResource(),
					fake.NewMockResource(),
				},
			},
			want: want{
				result: []ChildResource{
					fake.NewMockResource(fake.WithNamespaceName("", namespace)),
					fake.NewMockResource(fake.WithNamespaceName("", namespace)),
				},
			},
		},
		"KeepExistingNamespace": {
			args: args{
				cr: fake.NewMockResource(fake.WithNamespaceName("", namespace)),
				list: []ChildResource{
					fake.NewMockResource(fake.WithNamespaceName("", "olala")),
					fake.NewMockResource(),
				},
			},
			want: want{
				result: []ChildResource{
					fake.NewMockResource(fake.WithNamespaceName("", "olala")),
					fake.NewMockResource(fake.WithNamespaceName("", namespace)),
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := NewNamespacePatcher()
			got, err := p.Patch(tc.args.cr, tc.args.list)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}
		})
	}
}

func TestLabelPropagator(t *testing.T) {
	labels := map[string]string{
		"first": "val1",
		"sec":   "val2",
	}
	cases := map[string]struct {
		args
		want
	}{
		"AllNew": {
			args: args{
				cr: fake.NewMockResource(fake.WithLabels(labels)),
				list: []ChildResource{
					fake.NewMockResource(),
					fake.NewMockResource(),
				},
			},
			want: want{
				result: []ChildResource{
					fake.NewMockResource(fake.WithLabels(labels)),
					fake.NewMockResource(fake.WithLabels(labels)),
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := NewLabelPropagator()
			got, err := p.Patch(tc.args.cr, tc.args.list)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}
		})
	}
}

func TestParentLabelSetAdded(t *testing.T) {
	parent := fake.NewMockResource(fake.WithGVK(fake.MockParentGVK), fake.WithNamespaceName(name, namespace))
	cases := map[string]struct {
		args
		want
	}{
		"AllNew": {
			args: args{
				cr: parent,
				list: []ChildResource{
					fake.NewMockResource(),
					fake.NewMockResource(),
				},
			},
			want: want{
				result: []ChildResource{
					fake.NewMockResource(fake.WithLabels(stacks.ParentLabels(parent))),
					fake.NewMockResource(fake.WithLabels(stacks.ParentLabels(parent))),
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := NewParentLabelSetAdder()
			got, err := p.Patch(tc.args.cr, tc.args.list)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}
		})
	}
}
