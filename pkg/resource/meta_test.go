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
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/crossplane/templating-controller/pkg/resource/fake"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
)

const conditionedUnstructured = `
apiVersion: mock.crossplane.io/v1alpha1
kind: MockKind
status:
  conditions:
  - lastTransitionTime: "2020-02-18T15:07:11Z"
    reason: Successfully reconciled resource
    status: "True"
    type: Synced
`

func TestGetCondition(t *testing.T) {
	ti, _ := time.Parse(time.RFC3339, "2020-02-18T15:07:11Z")
	type args struct {
		u  interface{ UnstructuredContent() map[string]interface{} }
		ct v1alpha1.ConditionType
	}
	type want struct {
		c   v1alpha1.Condition
		err error
	}
	cases := map[string]struct {
		args
		want
	}{
		"GetExisting": {
			args: args{
				u:  fake.NewMockResource(fake.FromYAML([]byte(conditionedUnstructured))),
				ct: v1alpha1.TypeSynced,
			},
			want: want{
				c: v1alpha1.Condition{
					Type:               v1alpha1.TypeSynced,
					Status:             v1.ConditionTrue,
					LastTransitionTime: metav1.Time{Time: ti},
					Reason:             v1alpha1.ReasonReconcileSuccess,
					Message:            "",
				},
			},
		},
		"NotFound": {
			args: args{
				u:  fake.NewMockResource(fake.FromYAML([]byte(conditionedUnstructured))),
				ct: v1alpha1.TypeReady,
			},
			want: want{
				c: v1alpha1.Condition{Type: v1alpha1.TypeReady, Status: v1.ConditionUnknown},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := GetCondition(tc.args.u, tc.args.ct)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.c, got); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}
		})
	}
}

func TestSetCondition(t *testing.T) {
	ti, _ := time.Parse(time.RFC3339, "2020-02-18T15:07:11Z")
	type args struct {
		u interface{ UnstructuredContent() map[string]interface{} }
		c v1alpha1.Condition
	}
	type want struct {
		err error
	}
	cases := map[string]struct {
		args
		want
	}{
		"SetNew": {
			args: args{
				u: fake.NewMockResource(),
				c: v1alpha1.Condition{
					Type:               v1alpha1.TypeReady,
					Status:             v1.ConditionTrue,
					LastTransitionTime: metav1.Time{Time: ti},
					Reason:             v1alpha1.ReasonReconcileSuccess,
					Message:            "",
				},
			},
		},
		"SetExisting": {
			args: args{
				u: fake.NewMockResource(fake.FromYAML([]byte(conditionedUnstructured))),
				c: v1alpha1.Condition{
					Type:               v1alpha1.TypeSynced,
					Status:             v1.ConditionTrue,
					LastTransitionTime: metav1.Time{Time: ti},
					Reason:             v1alpha1.ReasonReconcileError,
					Message:            "i failed",
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := SetConditions(tc.args.u, tc.args.c)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}
			setCondition, _ := GetCondition(tc.args.u, tc.args.c.Type)
			if diff := cmp.Diff(tc.args.c, setCondition); diff != "" {
				t.Errorf("Reconcile(...): -want, +got:\n%s", diff)
			}
		})
	}
}
