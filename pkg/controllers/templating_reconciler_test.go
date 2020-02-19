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

package controllers

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	runtimefake "github.com/crossplane/crossplane-runtime/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/crossplane/templating-controller/pkg/resource"
	"github.com/crossplane/templating-controller/pkg/resource/fake"
)

const (
	fakeName      = "resname"
	fakeNamespace = "resnamespace"
)

var (
	timeNow = metav1.Now()
	errBoom = fmt.Errorf("boom")
)

func withNewParentResourceFunc(f func() resource.ParentResource) TemplatingReconcilerOption {
	return func(r *TemplatingReconciler) {
		r.newParentResource = f
	}
}

type MockParentResourceOption func(*fake.MockResource)

func withDeletionTimestamp(c *metav1.Time) MockParentResourceOption {
	return func(cr *fake.MockResource) {
		cr.SetDeletionTimestamp(c)
	}
}

func mockParentResource(opts ...MockParentResourceOption) *fake.MockResource {
	cr := &fake.MockResource{}
	for _, f := range opts {
		f(cr)
	}
	return cr
}

func TestReconcile(t *testing.T) {
	type args struct {
		kube client.Client
		opts []TemplatingReconcilerOption
	}
	type want struct {
		cr     resource.ParentResource
		result reconcile.Result
		err    error
	}
	cases := map[string]struct {
		args
		want
	}{
		"GetFailed": {
			args: args{
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(errBoom),
				},
			},
			want: want{
				err: errors.Wrap(errBoom, errGetResource),
			},
		},
		"Deleted": {
			args: args{
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
						cr := obj.(*fake.MockResource)
						cr.SetDeletionTimestamp(&timeNow)
						return nil
					}),
				},
			},
			want: want{},
		},
		"TemplatingFailed": {
			args: args{
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil),
					MockStatusUpdate: test.NewMockStatusUpdateFn(nil, func(obj runtime.Object) error {
						got := obj.(*fake.MockResource)
						gotCond, err := resource.GetCondition(got, v1alpha1.TypeSynced)
						if err != nil {
							t.Errorf("Reconcile(...): error getting condition\n%s", err.Error())
						}
						wantCond := v1alpha1.ReconcileError(errors.Wrap(errBoom, errTemplatingOperation))
						if diff := cmp.Diff(wantCond, gotCond); diff != "" {
							t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
						}
						return nil
					}),
				},
				opts: []TemplatingReconcilerOption{
					WithTemplatingEngine(resource.TemplatingEngineFunc(func(_ resource.ParentResource) ([]resource.ChildResource, error) {
						return nil, errBoom
					})),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: defaultShortWait},
			},
		},
		"ChildResourcePatchFailed": {
			args: args{
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil),
					MockStatusUpdate: test.NewMockStatusUpdateFn(nil, func(obj runtime.Object) error {
						got := obj.(*fake.MockResource)
						gotCond, err := resource.GetCondition(got, v1alpha1.TypeSynced)
						if err != nil {
							t.Errorf("Reconcile(...): error getting condition\n%s", err.Error())
						}
						wantCond := v1alpha1.ReconcileError(errors.Wrap(errBoom, errChildResourcePatchers))
						if diff := cmp.Diff(wantCond, gotCond); diff != "" {
							t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
						}
						return nil
					}),
				},
				opts: []TemplatingReconcilerOption{
					WithTemplatingEngine(&resource.NopTemplatingEngine{}),
					WithChildResourcePatcher(resource.ChildResourcePatcherFunc(func(_ resource.ParentResource, _ []resource.ChildResource) ([]resource.ChildResource, error) {
						return nil, errBoom
					})),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: defaultShortWait},
			},
		},
		"ApplyFailed": {
			args: args{
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil),
					MockPatch: test.NewMockPatchFn(nil, func(_ runtime.Object) error {
						return errBoom
					}),
					MockStatusUpdate: test.NewMockStatusUpdateFn(nil, func(obj runtime.Object) error {
						got := obj.(*fake.MockResource)
						gotCond, err := resource.GetCondition(got, v1alpha1.TypeSynced)
						if err != nil {
							t.Errorf("Reconcile(...): error getting condition\n%s", err.Error())
						}
						wantCond := v1alpha1.ReconcileError(errors.Wrap(errBoom, fmt.Sprintf("%s: %s/%s of type %s", errApply, fakeName, fakeNamespace, schema.EmptyObjectKind.GroupVersionKind().String())))
						if diff := cmp.Diff(wantCond, gotCond); diff != "" {
							t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
						}
						return nil
					}),
				},
				opts: []TemplatingReconcilerOption{
					WithTemplatingEngine(&resource.NopTemplatingEngine{}),
					WithChildResourcePatcher(resource.ChildResourcePatcherFunc(func(_ resource.ParentResource, _ []resource.ChildResource) ([]resource.ChildResource, error) {
						res := fake.NewMockResource()
						res.SetName(fakeName)
						res.SetNamespace(fakeNamespace)
						res.SetGroupVersionKind(schema.EmptyObjectKind.GroupVersionKind())
						return []resource.ChildResource{res}, nil
					})),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: defaultShortWait},
			},
		},
		"Success": {
			args: args{
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil),
					MockPatch: test.NewMockPatchFn(nil, func(_ runtime.Object) error {
						return errBoom
					}),
					MockStatusUpdate: test.NewMockStatusUpdateFn(nil, func(obj runtime.Object) error {
						got := obj.(*fake.MockResource)
						gotCond, err := resource.GetCondition(got, v1alpha1.TypeSynced)
						if err != nil {
							t.Errorf("Reconcile(...): error getting condition\n%s", err.Error())
						}
						wantCond := v1alpha1.ReconcileSuccess()
						if diff := cmp.Diff(wantCond, gotCond); diff != "" {
							t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
						}
						return nil
					}),
				},
				opts: []TemplatingReconcilerOption{
					WithTemplatingEngine(&resource.NopTemplatingEngine{}),
					WithChildResourcePatcher(resource.ChildResourcePatcherFunc(func(_ resource.ParentResource, _ []resource.ChildResource) ([]resource.ChildResource, error) {
						return nil, nil
					})),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: defaultLongWait},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mgr := &runtimefake.Manager{
				Client: tc.kube,
				Scheme: runtimefake.SchemeWith(&fake.MockResource{}),
			}
			tc.args.opts = append(tc.args.opts, withNewParentResourceFunc(func() resource.ParentResource {
				cr := &fake.MockResource{}
				cr.SetGroupVersionKind(schema.EmptyObjectKind.GroupVersionKind())
				return cr
			}))
			r := NewTemplatingReconciler(mgr, (&fake.MockResource{}).GroupVersionKind(), tc.args.opts...)
			result, err := r.Reconcile(reconcile.Request{})

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, result); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}

		})
	}
}

func TestApply(t *testing.T) {
	type args struct {
		kube client.Client
		o    resource.ChildResource
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		args
		want
	}{
		"NotFoundCreate": {
			args: args{
				kube: &test.MockClient{
					MockGet:    test.NewMockGetFn(kerrors.NewNotFound(schema.GroupResource{}, fakeName)),
					MockCreate: test.NewMockCreateFn(errBoom),
				},
				o: fake.NewMockResource(),
			},
			want: want{
				err: errors.Wrap(errBoom, errCreateChildResource),
			},
		},
		"GetFailed": {
			args: args{
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(errBoom),
				},
				o: fake.NewMockResource(),
			},
			want: want{
				err: errors.Wrap(errBoom, errGetChildResource),
			},
		},
		"Success": {
			args: args{
				kube: &test.MockClient{
					MockGet:   test.NewMockGetFn(nil),
					MockPatch: test.NewMockPatchFn(nil),
				},
				o: fake.NewMockResource(),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := Apply(context.Background(), tc.args.kube, tc.args.o)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}
		})
	}
}
