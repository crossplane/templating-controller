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

	"github.com/pkg/errors"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/api/filesys"
	"sigs.k8s.io/kustomize/api/krusty"
	kustomizeapi "sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/yaml"

	"github.com/muvaf/configuration-stacks/pkg/resource"
)

const (
	errPatch              = "patch call of KustomizationPatcher failed"
	errOverlayPreparation = "overlay preparation failed"
	errKustomizationCall  = "kustomization call failed"
)

// NewKustomizeOperation returns a KustomizeOperation object. rootPath should
// point to the folder where your base kustomization.yaml resides and patcher
// is the chain of KustomizationPatcher that makes modifications of Kustomization
// object.
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

	// Patcher contains the modifications that you'd like to make to
	// the overlay Kustomization object before calling kustomize.
	Patcher resource.KustomizationPatcherChain
}

func (o *KustomizeOperation) Run(cr resource.ParentResource) ([]resource.ChildResource, error) {
	k := &kustomizeapi.Kustomization{}
	if err := o.Patcher.Patch(cr, k); err != nil {
		return nil, errors.Wrap(err, errPatch)
	}
	dir, err := o.prepareOverlay(cr, k)
	if err != nil {
		return nil, errors.Wrap(err, errOverlayPreparation)
	}
	kustomizer := krusty.MakeKustomizer(filesys.MakeFsOnDisk(), krusty.MakeDefaultOptions())
	resMap, err := kustomizer.Run(dir)
	if err != nil {
		return nil, errors.Wrap(err, errKustomizationCall)
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

func (o *KustomizeOperation) prepareOverlay(cr resource.ParentResource, k *kustomizeapi.Kustomization) (string, error) {
	// Kustomize does not work with symlinked paths, so, we're using its own
	// temp directory generation function.
	tempConfirmedDir, err := filesys.NewTmpConfirmedDir()
	if err != nil {
		return "", err
	}
	tempDir := string(tempConfirmedDir)
	defer os.RemoveAll(string(tempDir))
	crYAML, err := yaml.Marshal(cr)
	if err != nil {
		return "", err
	}
	if err := ioutil.WriteFile(fmt.Sprintf("%s/cr.yaml", tempDir), crYAML, os.ModePerm); err != nil {
		return "", err
	}

	// Kustomize doesn't work with absolute paths, all paths have to be relative
	// to the root path of the folder where kustomize points to.
	absPath, err := filepath.Abs(o.ResourcePath)
	if err != nil {
		return "", err
	}
	relPath, err := filepath.Rel(tempDir, absPath)
	if err != nil {
		return "", err
	}
	k.Resources = append(k.Resources, []string{relPath, "cr.yaml"}...)
	yamlData, err := yaml.Marshal(k)
	if err != nil {
		return "", err
	}
	if err := ioutil.WriteFile(fmt.Sprintf("%s/kustomization.yaml", tempDir), yamlData, os.ModePerm); err != nil {
		return "", err
	}
	return tempDir, nil
}
