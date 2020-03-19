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

package kustomize

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/yaml"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/crossplane/crossplane/apis/stacks/v1alpha1"

	"github.com/crossplane/templating-controller/pkg/resource"
)

const testYAMLDir = "../../../test/kustomize"

func TestEngine_Run(t *testing.T) {
	errBoom := errors.New("stay healthy")
	kcData, err := ioutil.ReadFile(filepath.Join(testYAMLDir, "test-overlays.yaml"))
	if err != nil {
		panic(fmt.Sprintf("cannot read %s", "test-overlays.yaml"))
	}
	kc := &v1alpha1.KustomizeEngineConfiguration{}
	if err := yaml.Unmarshal(kcData, kc); err != nil {
		panic(fmt.Sprintf("cannot parse %s", "test-overlays.yaml"))
	}

	type args struct {
		cr resource.ParentResource
		e  resource.TemplatingEngine
	}
	type want struct {
		result []resource.ChildResource
		err    error
	}

	cases := map[string]struct {
		args
		want
	}{
		"PatcherFailed": {
			args: args{
				cr: &unstructured.Unstructured{},
				e: &engine{
					Patchers: []Patcher{PatcherFunc(func(resource.ParentResource, *types.Kustomization) error {
						return errBoom
					})},
				},
			},
			want: want{
				err: errors.Wrap(errBoom, errPatch),
			},
		},
		"OverlayGeneratorFailed": {
			args: args{
				cr: &unstructured.Unstructured{},
				e: &engine{
					OverlayGenerators: []OverlayGenerator{OverlayGeneratorFunc(func(cr resource.ParentResource, k *types.Kustomization) ([]OverlayFile, error) {
						return nil, errBoom
					})},
				},
			},
			want: want{
				err: errors.Wrap(errBoom, errOverlayGeneration),
			},
		},
		"Success": {
			args: args{
				cr: parse(filepath.Join(testYAMLDir, "test-cr.yaml")),
				e:  NewKustomizeEngine(nil, WithResourcePath(filepath.Join(testYAMLDir, "resources")), WithOverlayGenerator(NewPatchOverlayGenerator(kc.Overlays))),
			},
			want: want{
				result: []resource.ChildResource{parse(filepath.Join(testYAMLDir, "want.yaml"))},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := tc.args.e.Run(tc.args.cr)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("Run(...): -want, +got:\n%s", diff)
			}
			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("Run(...): -want, +got:\n%s", diff)
			}
		})
	}
}

func parse(path string) *unstructured.Unstructured {
	resultData, err := ioutil.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("cannot read %s", path))
	}
	dec := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(resultData), 4096)
	u := &unstructured.Unstructured{}
	if err := dec.Decode(u); err != nil {
		panic(fmt.Sprintf("cannot parse %s", path))
	}
	return u
}
