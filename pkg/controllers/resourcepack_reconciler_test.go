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
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplaneio/crossplane-runtime/apis/core/v1alpha1"
	runtimefake "github.com/crossplaneio/crossplane-runtime/pkg/resource/fake"
	"github.com/crossplaneio/crossplane-runtime/pkg/test"

	"github.com/muvaf/crossplane-resourcepacks/pkg/resource"
	"github.com/muvaf/crossplane-resourcepacks/pkg/resource/fake"
)

type MockParentResourceOption func(*fake.MockParentResource)

func WithConditions(c ...v1alpha1.Condition) MockParentResourceOption {
	return func(cr *fake.MockParentResource) {
		cr.SetConditions(c...)
	}
}

func mockParentResource(opts ...MockParentResourceOption) *fake.MockParentResource {
	cr := &fake.MockParentResource{}
	for _, f := range opts {
		f(cr)
	}
	return cr
}

func TestReconcile(t *testing.T) {
	errBoom := fmt.Errorf("boom")
	type args struct {
		cr   resource.ParentResource
		kube client.Client
	}
	type want struct {
		cr  resource.ParentResource
		err error
	}
	cases := map[string]struct {
		args
		want
	}{
		"GetFailed": {
			args: args{
				cr: mockParentResource(),
				kube: &test.MockClient{
					MockGet: test.NewMockGetFn(errBoom),
				},
			},
			want: want{
				err: errors.Wrap(errBoom, errGetResource),
				cr:  mockParentResource(),
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mgr := &runtimefake.MockManager{
				Client: tc.kube,
				Scheme: runtimefake.MockSchemeWith(&fake.MockParentResource{}),
			}
			r := NewResourcePackReconciler(mgr, runtimefake.MockGVK(&fake.MockParentResource{}))
			_, err := r.Reconcile(reconcile.Request{})

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}

			if diff := cmp.Diff(tc.want.cr, tc.args.cr); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}
		})
	}
}
