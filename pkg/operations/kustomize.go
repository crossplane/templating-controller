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

package operations

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/api/filesys"
	"sigs.k8s.io/kustomize/api/krusty"
	kustomizeapi "sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/yaml"

	"github.com/muvaf/configuration-stacks/pkg/resource"
)

func NewKustomizeOperation(rootPath string, patcher resource.KustomizationPatcherChain) *KustomizeOperation {
	return &KustomizeOperation{
		ResourcePath: rootPath,
		Patcher:      patcher,
	}
}

type KustomizeOperation struct {
	// ResourcePath is the folder that the base resources reside in the
	// filesystem. It should be given as absolute path.
	ResourcePath string

	Patcher resource.KustomizationPatcherChain
}

// RunKustomize creates a temporary directory that contains ParentResource YAML
// and main kustomization.yaml file that refers to the original resources folder.
// The temporary folder is deleted after the function exits.
func (o *KustomizeOperation) RunKustomize(cr resource.ParentResource) ([]resource.ChildResource, error) {
	// Kustomize does not work with symlinked paths, so, we're using its own
	// temp directory generation function.
	tempConfirmedDir, err := filesys.NewTmpConfirmedDir()
	if err != nil {
		return nil, err
	}
	tempDir := string(tempConfirmedDir)
	defer os.RemoveAll(string(tempDir))
	crYAML, err := yaml.Marshal(cr)
	if err != nil {
		return nil, err
	}
	if err := ioutil.WriteFile(fmt.Sprintf("%s/cr.yaml", tempDir), crYAML, os.ModePerm); err != nil {
		return nil, err
	}

	// Kustomize doesn't work with absolute paths, all paths have to be relative
	// to the root path of the folder where kustomize points to.
	absPath, err := filepath.Abs(o.ResourcePath)
	if err != nil {
		return nil, err
	}
	relPath, err := filepath.Rel(tempDir, absPath)
	if err != nil {
		return nil, err
	}
	k := &kustomizeapi.Kustomization{
		Resources: []string{
			relPath,
			"cr.yaml",
		},
	}

	if err := o.Patcher.Patch(cr, k); err != nil {
		return nil, err
	}
	yamlData, err := yaml.Marshal(k)
	if err != nil {
		return nil, err
	}
	if err := ioutil.WriteFile(fmt.Sprintf("%s/kustomization.yaml", tempDir), yamlData, os.ModePerm); err != nil {
		return nil, err
	}
	kustomizer := krusty.MakeKustomizer(filesys.MakeFsOnDisk(), krusty.MakeDefaultOptions())
	resMap, err := kustomizer.Run(tempDir)
	if err != nil {
		return nil, err
	}
	var objects []resource.ChildResource
	for _, res := range resMap.Resources() {
		// ParentResource is written to kustomization directory only to be used
		// for value retrieval.
		if res.GetKind() == cr.GetObjectKind().GroupVersionKind().Kind {
			continue
		}
		u := &unstructured.Unstructured{}
		u.SetUnstructuredContent(res.Map())
		objects = append(objects, u)
	}

	return objects, nil
}
