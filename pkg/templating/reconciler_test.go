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
	runtimeresource "github.com/crossplane/crossplane-runtime/pkg/resource"
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
	errBoom = fmt.Errorf("boom")
)

func withNewParentResourceFunc(f func() resource.ParentResource) ReconcilerOption {
	return func(r *Reconciler) {
		r.newParentResource = f
	}
}

func TestReconcile(t *testing.T) {
	type args struct {
		kube client.Client
		opts []ReconcilerOption
	}
	type want struct {
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
				opts: []ReconcilerOption{
					WithEngine(EngineFunc(func(_ resource.ParentResource) ([]resource.ChildResource, error) {
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
				opts: []ReconcilerOption{
					WithEngine(&NopEngine{}),
					WithChildResourcePatcher(ChildResourcePatcherFunc(func(_ resource.ParentResource, _ []resource.ChildResource) ([]resource.ChildResource, error) {
						return nil, errBoom
					})),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: defaultShortWait},
			},
		},
		"DeleterFailed": {
			args: args{
				kube: &test.MockClient{
					MockGet: func(_ context.Context, _ client.ObjectKey, obj runtime.Object) error {
						mobj, _ := obj.(metav1.Object)
						now := metav1.Now()
						mobj.SetDeletionTimestamp(&now)
						return nil
					},
					MockStatusUpdate: test.NewMockStatusUpdateFn(nil, func(obj runtime.Object) error {
						got := obj.(*fake.MockResource)
						gotCond, err := resource.GetCondition(got, v1alpha1.TypeSynced)
						if err != nil {
							t.Errorf("Reconcile(...): error getting condition\n%s", err.Error())
						}
						wantCond := v1alpha1.ReconcileError(errors.Wrap(errBoom, errDeleter))
						if diff := cmp.Diff(wantCond, gotCond); diff != "" {
							t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
						}
						return nil
					}),
				},
				opts: []ReconcilerOption{
					WithEngine(&NopEngine{}),
					WithChildResourcePatcher(ChildResourcePatcherFunc(func(_ resource.ParentResource, list []resource.ChildResource) ([]resource.ChildResource, error) {
						return list, nil
					})),
					WithChildResourceDeleter(ChildResourceDeleterFunc(func(_ context.Context, _ []resource.ChildResource) ([]resource.ChildResource, error) {
						return nil, errBoom
					})),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: defaultShortWait},
			},
		},
		"StillDeleting": {
			args: args{
				kube: &test.MockClient{
					MockGet: func(_ context.Context, _ client.ObjectKey, obj runtime.Object) error {
						mobj, _ := obj.(metav1.Object)
						now := metav1.Now()
						mobj.SetDeletionTimestamp(&now)
						return nil
					},
					MockStatusUpdate: test.NewMockStatusUpdateFn(nil, func(obj runtime.Object) error {
						got := obj.(*fake.MockResource)
						gotCond, err := resource.GetCondition(got, v1alpha1.TypeSynced)
						if err != nil {
							t.Errorf("Reconcile(...): error getting condition\n%s", err.Error())
						}
						wantCond := v1alpha1.ReconcileSuccess().WithMessage(msgWaitingForDeletion)
						if diff := cmp.Diff(wantCond, gotCond); diff != "" {
							t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
						}
						return nil
					}),
				},
				opts: []ReconcilerOption{
					WithEngine(&NopEngine{}),
					WithChildResourcePatcher(ChildResourcePatcherFunc(func(_ resource.ParentResource, list []resource.ChildResource) ([]resource.ChildResource, error) {
						return list, nil
					})),
					WithChildResourceDeleter(ChildResourceDeleterFunc(func(_ context.Context, _ []resource.ChildResource) ([]resource.ChildResource, error) {
						return []resource.ChildResource{fake.NewMockResource()}, nil
					})),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: tinyWait},
			},
		},
		"DeletionCompletedFinalizerFailed": {
			args: args{
				kube: &test.MockClient{
					MockGet: func(_ context.Context, _ client.ObjectKey, obj runtime.Object) error {
						mobj, _ := obj.(metav1.Object)
						now := metav1.Now()
						mobj.SetDeletionTimestamp(&now)
						return nil
					},
					MockStatusUpdate: test.NewMockStatusUpdateFn(nil, func(obj runtime.Object) error {
						got := obj.(*fake.MockResource)
						gotCond, err := resource.GetCondition(got, v1alpha1.TypeSynced)
						if err != nil {
							t.Errorf("Reconcile(...): error getting condition\n%s", err.Error())
						}
						wantCond := v1alpha1.ReconcileError(errors.Wrap(errBoom, errRemoveFinalizer))
						if diff := cmp.Diff(wantCond, gotCond); diff != "" {
							t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
						}
						return nil
					}),
				},
				opts: []ReconcilerOption{
					WithEngine(&NopEngine{}),
					WithChildResourcePatcher(ChildResourcePatcherFunc(func(_ resource.ParentResource, list []resource.ChildResource) ([]resource.ChildResource, error) {
						return list, nil
					})),
					WithChildResourceDeleter(ChildResourceDeleterFunc(func(_ context.Context, _ []resource.ChildResource) ([]resource.ChildResource, error) {
						return nil, nil
					})),
					WithFinalizer(runtimeresource.FinalizerFns{RemoveFinalizerFn: func(_ context.Context, _ runtimeresource.Object) error {
						return errBoom
					}}),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: defaultShortWait},
			},
		},
		"DeletionCompletedSuccess": {
			args: args{
				kube: &test.MockClient{
					MockGet: func(_ context.Context, _ client.ObjectKey, obj runtime.Object) error {
						mobj, _ := obj.(metav1.Object)
						now := metav1.Now()
						mobj.SetDeletionTimestamp(&now)
						return nil
					},
				},
				opts: []ReconcilerOption{
					WithEngine(&NopEngine{}),
					WithChildResourcePatcher(ChildResourcePatcherFunc(func(_ resource.ParentResource, list []resource.ChildResource) ([]resource.ChildResource, error) {
						return list, nil
					})),
					WithChildResourceDeleter(ChildResourceDeleterFunc(func(_ context.Context, _ []resource.ChildResource) ([]resource.ChildResource, error) {
						return nil, nil
					})),
					WithFinalizer(runtimeresource.FinalizerFns{RemoveFinalizerFn: func(_ context.Context, _ runtimeresource.Object) error {
						return nil
					}}),
				},
			},
			want: want{
				result: reconcile.Result{Requeue: false},
			},
		},
		"FinalizerAdditionFailed": {
			args: args{
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(nil),
					MockStatusUpdate: test.NewMockStatusUpdateFn(nil, func(obj runtime.Object) error {
						got := obj.(*fake.MockResource)
						gotCond, err := resource.GetCondition(got, v1alpha1.TypeSynced)
						if err != nil {
							t.Errorf("Reconcile(...): error getting condition\n%s", err.Error())
						}
						wantCond := v1alpha1.ReconcileError(errors.Wrap(errBoom, errAddFinalizer))
						if diff := cmp.Diff(wantCond, gotCond); diff != "" {
							t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
						}
						return nil
					}),
				},
				opts: []ReconcilerOption{
					WithEngine(&NopEngine{}),
					WithChildResourcePatcher(ChildResourcePatcherFunc(func(_ resource.ParentResource, list []resource.ChildResource) ([]resource.ChildResource, error) {
						return list, nil
					})),
					WithFinalizer(runtimeresource.FinalizerFns{AddFinalizerFn: func(_ context.Context, _ runtimeresource.Object) error {
						return errBoom
					}}),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: defaultShortWait},
			},
		},
		"ApplyFailed": {
			args: args{
				kube: &test.MockClient{
					MockGet:    test.NewMockGetFn(nil),
					MockUpdate: test.NewMockUpdateFn(nil),
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
				opts: []ReconcilerOption{
					WithEngine(&NopEngine{}),
					WithChildResourcePatcher(ChildResourcePatcherFunc(func(_ resource.ParentResource, _ []resource.ChildResource) ([]resource.ChildResource, error) {
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
					MockGet:    test.NewMockGetFn(nil),
					MockUpdate: test.NewMockUpdateFn(nil),
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
				opts: []ReconcilerOption{
					WithEngine(&NopEngine{}),
					WithChildResourcePatcher(ChildResourcePatcherFunc(func(_ resource.ParentResource, _ []resource.ChildResource) ([]resource.ChildResource, error) {
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
				return fake.NewMockResource(fake.WithGVK(schema.EmptyObjectKind.GroupVersionKind()))
			}))
			r := NewReconciler(mgr, (&fake.MockResource{}).GroupVersionKind(), tc.args.opts...)
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
