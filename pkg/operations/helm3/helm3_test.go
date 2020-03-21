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

package helm3

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/crossplane/templating-controller/pkg/resource"
)

const testYAMLDir = "../../../test/helm3"

var errContains = cmp.Comparer(func(a, b error) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return strings.Contains(a.Error(), b.Error()) || strings.Contains(b.Error(), a.Error())
})

func TestRun(t *testing.T) {
	testYaml, err := ioutil.ReadFile(filepath.Join(testYAMLDir, "test-cr.yaml"))
	if err != nil {
		panic("test-cr.yaml is deleted")
	}
	res, err := parse(testYaml)
	if err != nil {
		panic("cannot parse test-cr.yaml")
	}
	parentCR := res[0].(resource.ParentResource)

	resultYaml, err := ioutil.ReadFile(filepath.Join(testYAMLDir, "want.yaml"))
	if err != nil {
		panic("want.yaml is deleted")
	}
	results, err := parse(resultYaml)
	if err != nil {
		panic("cannot parse want.yaml")
	}

	type args struct {
		cr resource.ParentResource
		e  resource.TemplatingEngine
	}
	type want struct {
		result      []resource.ChildResource
		errContains error
	}

	cases := map[string]struct {
		args
		want
	}{
		"SpecNotMap": {
			args: args{
				cr: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"spec": "olala",
					},
				},
				e: NewHelm3Engine(),
			},
			want: want{
				errContains: errors.New(errSpecCast),
			},
		},
		"TemplateFailed": {
			args: args{
				cr: &unstructured.Unstructured{},
				e:  NewHelm3Engine(WithResourcePath("/i-dont-exist")),
			},
			want: want{
				errContains: errors.Wrap(fmt.Errorf(""), errHelm3Template),
			},
		},
		"Success": {
			args: args{
				cr: parentCR,
				e:  NewHelm3Engine(WithResourcePath(filepath.Join(testYAMLDir, "helm-chart"))),
			},
			want: want{
				result:      results,
				errContains: nil,
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := tc.args.e.Run(tc.args.cr)
			if diff := cmp.Diff(tc.want.errContains, err, errContains); diff != "" {
				// NOTE(muvaf): Some functions return errors from syscalls , we
				// are not able to construct them.
				t.Errorf("Run(...): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("Run(...): -want, +got:\n%s", diff)
			}
		})
	}
}
