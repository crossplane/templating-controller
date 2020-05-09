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

package templating

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/crossplane/crossplane/pkg/stacks"

	"github.com/crossplane/templating-controller/pkg/resource"
	"github.com/crossplane/templating-controller/pkg/resource/fake"
)

const (
	name      = "fakename"
	namespace = "fakenamespace"
)

var (
	_ ChildResourcePatcher = OwnerReferenceAdder{}
	_ ChildResourcePatcher = DefaultingAnnotationRemover{}
	_ ChildResourcePatcher = NamespacePatcher{}
	_ ChildResourcePatcher = LabelPropagator{}
	_ ChildResourcePatcher = ParentLabelSetAdder{}

	_ ChildResourceDeleter = &APIOrderedDeleter{}
)

type args struct {
	cr   resource.ParentResource
	list []resource.ChildResource
}

type want struct {
	result []resource.ChildResource
	err    error
}

func TestDefaultingAnnotationRemover(t *testing.T) {
	cases := map[string]struct {
		args
		want
	}{
		"KeepAnnotated": {
			args: args{
				cr: fake.NewMockResource(),
				list: []resource.ChildResource{
					fake.NewMockResource(fake.WithAdditionalAnnotations(map[string]string{v1alpha1.AnnotationDefaultClassKey: v1alpha1.AnnotationDefaultClassValue})),
					fake.NewMockResource(fake.WithAdditionalAnnotations(map[string]string{v1alpha1.AnnotationDefaultClassKey: v1alpha1.AnnotationDefaultClassValue})),
				},
			},
			want: want{
				result: []resource.ChildResource{
					fake.NewMockResource(fake.WithAdditionalAnnotations(map[string]string{v1alpha1.AnnotationDefaultClassKey: v1alpha1.AnnotationDefaultClassValue})),
					fake.NewMockResource(fake.WithAdditionalAnnotations(map[string]string{v1alpha1.AnnotationDefaultClassKey: v1alpha1.AnnotationDefaultClassValue})),
				},
			},
		},
		"RemoveAnnotation": {
			args: args{
				cr: fake.NewMockResource(fake.WithAdditionalAnnotations(map[string]string{RemoveDefaultAnnotationsKey: RemoveDefaultAnnotationsTrueValue})),
				list: []resource.ChildResource{
					fake.NewMockResource(fake.WithAdditionalAnnotations(map[string]string{v1alpha1.AnnotationDefaultClassKey: v1alpha1.AnnotationDefaultClassValue})),
					fake.NewMockResource(),
				},
			},
			want: want{
				result: []resource.ChildResource{
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
				t.Errorf("Patch(...): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("Patch(...): -want, +got:\n%s", diff)
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
				list: []resource.ChildResource{
					fake.NewMockResource(),
					fake.NewMockResource(),
				},
			},
			want: want{
				result: []resource.ChildResource{
					fake.NewMockResource(fake.WithControllerRef(parent, parent.GroupVersionKind())),
					fake.NewMockResource(fake.WithControllerRef(parent, parent.GroupVersionKind())),
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := NewOwnerReferenceAdder()
			got, err := p.Patch(tc.args.cr, tc.args.list)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Patch(...): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("Patch(...): -want, +got:\n%s", diff)
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
				list: []resource.ChildResource{
					fake.NewMockResource(),
					fake.NewMockResource(),
				},
			},
			want: want{
				result: []resource.ChildResource{
					fake.NewMockResource(fake.WithNamespaceName("", namespace)),
					fake.NewMockResource(fake.WithNamespaceName("", namespace)),
				},
			},
		},
		"KeepExistingNamespace": {
			args: args{
				cr: fake.NewMockResource(fake.WithNamespaceName("", namespace)),
				list: []resource.ChildResource{
					fake.NewMockResource(fake.WithNamespaceName("", "olala")),
					fake.NewMockResource(),
				},
			},
			want: want{
				result: []resource.ChildResource{
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
				t.Errorf("Patch(...): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("Patch(...): -want, +got:\n%s", diff)
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
				cr: fake.NewMockResource(fake.WithAdditionalLabels(labels)),
				list: []resource.ChildResource{
					fake.NewMockResource(),
					fake.NewMockResource(),
				},
			},
			want: want{
				result: []resource.ChildResource{
					fake.NewMockResource(fake.WithAdditionalLabels(labels)),
					fake.NewMockResource(fake.WithAdditionalLabels(labels)),
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := NewLabelPropagator()
			got, err := p.Patch(tc.args.cr, tc.args.list)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Patch(...): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("Patch(...): -want, +got:\n%s", diff)
			}
		})
	}
}

func TestParentLabelSetAdder(t *testing.T) {
	parent := fake.NewMockResource(fake.WithGVK(fake.MockParentGVK), fake.WithNamespaceName(name, namespace))
	cases := map[string]struct {
		args
		want
	}{
		"AllNew": {
			args: args{
				cr: parent,
				list: []resource.ChildResource{
					fake.NewMockResource(),
					fake.NewMockResource(),
				},
			},
			want: want{
				result: []resource.ChildResource{
					fake.NewMockResource(fake.WithAdditionalLabels(stacks.ParentLabels(parent))),
					fake.NewMockResource(fake.WithAdditionalLabels(stacks.ParentLabels(parent))),
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := NewParentLabelSetAdder()
			got, err := p.Patch(tc.args.cr, tc.args.list)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Patch(...): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("Patch(...): -want, +got:\n%s", diff)
			}
		})
	}
}

func TestAPIOrderedDeleter_Delete(t *testing.T) {
	type args struct {
		kube client.Client
		cr   resource.ParentResource
		list []resource.ChildResource
	}
	type want struct {
		deleting []resource.ChildResource
		err      error
	}

	cases := map[string]struct {
		reason string
		args
		want
	}{
		"ChooseHighestPriorityFirst": {
			reason: "Deletion should start with the resources with highest priority",
			args: args{
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil),
					MockDelete: func(_ context.Context, obj runtime.Object, _ ...client.DeleteOption) error {
						if mobj, ok := obj.(metav1.Object); ok {
							if mobj.GetAnnotations()[DeletionPriorityAnnotationKey] != "99" {
								t.Errorf("unexpected delete call is made")
							}
						}
						return nil
					},
				},
				list: []resource.ChildResource{
					fake.NewMockResource(fake.WithAdditionalAnnotations(map[string]string{DeletionPriorityAnnotationKey: "99"})),
					fake.NewMockResource(fake.WithAdditionalAnnotations(map[string]string{DeletionPriorityAnnotationKey: "49"})),
					fake.NewMockResource(),
				},
			},
			want: want{
				deleting: []resource.ChildResource{
					fake.NewMockResource(fake.WithAdditionalAnnotations(map[string]string{DeletionPriorityAnnotationKey: "99"})),
				},
			},
		},
		"ShouldDeleteSecondHighest": {
			reason: "Deletion should be called for the resources with second highest priority when the highest one is already deleted",
			args: args{
				kube: &test.MockClient{
					MockGet: func(_ context.Context, _ client.ObjectKey, obj runtime.Object) error {
						mobj, _ := obj.(metav1.Object)
						if mobj.GetAnnotations()[DeletionPriorityAnnotationKey] == "99" {
							return kerrors.NewNotFound(schema.GroupResource{}, "")
						}
						return nil
					},
					MockDelete: func(_ context.Context, obj runtime.Object, _ ...client.DeleteOption) error {
						mobj, _ := obj.(metav1.Object)
						if mobj.GetAnnotations()[DeletionPriorityAnnotationKey] != "49" {
							t.Errorf("unexpected delete call is made")
						}
						return nil
					},
				},
				list: []resource.ChildResource{
					fake.NewMockResource(fake.WithAdditionalAnnotations(map[string]string{DeletionPriorityAnnotationKey: "99"})),
					fake.NewMockResource(fake.WithAdditionalAnnotations(map[string]string{DeletionPriorityAnnotationKey: "49"})),
					fake.NewMockResource(),
				},
			},
			want: want{
				deleting: []resource.ChildResource{
					fake.NewMockResource(fake.WithAdditionalAnnotations(map[string]string{DeletionPriorityAnnotationKey: "49"})),
				},
			},
		},
		"NegativePriority": {
			reason: "If a resource has a negative priority and the rest does not have any priority, it should be deleted last",
			args: args{
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil),
					MockDelete: func(_ context.Context, obj runtime.Object, _ ...client.DeleteOption) error {
						mobj, _ := obj.(metav1.Object)
						if mobj.GetAnnotations()[DeletionPriorityAnnotationKey] != "" {
							t.Errorf("unexpected delete call is made")
						}
						return nil
					},
				},
				list: []resource.ChildResource{
					fake.NewMockResource(),
					fake.NewMockResource(fake.WithAdditionalAnnotations(map[string]string{DeletionPriorityAnnotationKey: "-1"})),
					fake.NewMockResource(),
				},
			},
			want: want{
				deleting: []resource.ChildResource{
					fake.NewMockResource(),
					fake.NewMockResource(),
				},
			},
		},
		"AnnotationIsNotInt": {
			reason: "It should return error if the priority annotation is not integer",
			args: args{
				list: []resource.ChildResource{
					fake.NewMockResource(fake.WithAdditionalAnnotations(map[string]string{DeletionPriorityAnnotationKey: "ola"})),
				},
			},
			want: want{
				err: errors.Wrap(errors.New("strconv.ParseInt: parsing \"ola\": invalid syntax"), errPriorityToInt),
			},
		},
		"GetFailed": {
			reason: "It should return error if get operation has failed",
			args: args{
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(errBoom),
				},
				list: []resource.ChildResource{
					fake.NewMockResource(),
				},
			},
			want: want{
				err: errors.Wrap(errBoom, errGetChildResource),
			},
		},
		"DeletionFailedIfNotOwner": {
			reason: "It should return error if the owner of the deleted object is not given parent",
			args: args{
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil),
				},
				cr: fake.NewMockResource(fake.WithUID("foo")),
				list: []resource.ChildResource{
					fake.NewMockResource(fake.WithControllerRef(fake.NewMockResource(fake.WithUID("bar")), schema.EmptyObjectKind.GroupVersionKind())),
				},
			},
			want: want{
				err: errors.New(errNotController),
			},
		},
		"DeletionFailed": {
			reason: "It should return error if deletion has failed",
			args: args{
				kube: &test.MockClient{
					MockGet:    test.NewMockGetFn(nil),
					MockDelete: test.NewMockDeleteFn(errBoom),
				},
				list: []resource.ChildResource{
					fake.NewMockResource(),
				},
			},
			want: want{
				err: errors.Wrap(errBoom, errDeleteChildResource),
			},
		},
		"ShouldDeleteAll": {
			reason: "Deletion should be called for all the resources if their priority order is the same",
			args: args{
				kube: &test.MockClient{
					MockGet:    test.NewMockGetFn(nil),
					MockDelete: test.NewMockDeleteFn(nil),
				},
				list: []resource.ChildResource{
					fake.NewMockResource(),
					fake.NewMockResource(),
				},
			},
			want: want{
				deleting: []resource.ChildResource{
					fake.NewMockResource(),
					fake.NewMockResource(),
				},
			},
		},
		"ReturnEmptyListIfAllDeleted": {
			reason: "When all the resources are already deleted, it should return an empty list",
			args: args{
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(kerrors.NewNotFound(schema.GroupResource{}, "")),
				},
				list: []resource.ChildResource{
					fake.NewMockResource(),
					fake.NewMockResource(),
				},
			},
			want: want{
				deleting: []resource.ChildResource{},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := NewAPIOrderedDeleter(tc.args.kube)
			deleting, err := d.Delete(context.Background(), tc.args.cr, tc.args.list)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Delete(...): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.deleting, deleting); diff != "" {
				t.Errorf("Delete(...): -want, +got:\n%s", diff)
			}
		})
	}

}
