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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/api/filesys"
	"sigs.k8s.io/kustomize/api/krusty"
	kustomizeapi "sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/yaml"

	"github.com/muvaf/configuration-stacks/pkg/resource"
)

func NewKustomizeOperation(rootPath string, overrider resource.KustomizeOverrider) *KustomizeOperation {
	return &KustomizeOperation{
		RootPath:     rootPath,
		overrider: overrider,
	}
}

type KustomizeOperation struct {
	// RootPath is the folder that the main kustomization.yaml file resides.
	RootPath string

	overrider resource.KustomizeOverrider
}

func (o *KustomizeOperation) RunKustomize(cr resource.ParentResource) ([]resource.ChildResource, error) {
	kustomizationFilePath := fmt.Sprintf("%s/kustomization.yaml", o.RootPath)
	k := &kustomizeapi.Kustomization{}
	data, err := ioutil.ReadFile(kustomizationFilePath)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, k); err != nil {
		return nil, err
	}
	if o.overrider != nil {
		o.overrider.Process(cr, k)
	}
	yamlData, err := yaml.Marshal(k)
	if err != nil {
		return nil, err
	}
	if err := ioutil.WriteFile(kustomizationFilePath, yamlData, os.ModePerm); err != nil {
		return nil, err
	}
	kustomizer := krusty.MakeKustomizer(filesys.MakeFsOnDisk(), krusty.MakeDefaultOptions())
	resMap, err := kustomizer.Run(o.RootPath)
	if err != nil {
		return nil, err
	}
	var objects []resource.ChildResource
	for _, res := range resMap.Resources() {
		u := &unstructured.Unstructured{}
		u.SetUnstructuredContent(res.Map())
		objects = append(objects, u)
	}
	return objects, nil
}
