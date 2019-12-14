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

package kustomize

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/kustomize/api/filesys"
	"sigs.k8s.io/kustomize/api/krusty"
	kustomizeapi "sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/yaml"

	"github.com/muvaf/crossplane-resourcepacks/pkg/resource"
)

const (
	defaultRootPath       = "resources"
	templateFileName      = "kustomization.yaml.tmpl"
	kustomizationFileName = "kustomization.yaml"
	temporaryCRFileName   = "cr.yaml"

	errPatch              = "patch call failed"
	errOverlayPreparation = "overlay preparation failed"
	errKustomizeCall      = "kustomize call failed"
)

// KustomizeOption is used to manipulate default KustomizeEngine parameters.
type KustomizeOption func(*KustomizeEngine)

// WithResourcePath allows you to specify a kustomization folder other than default.
func WithResourcePath(path string) KustomizeOption {
	return func(ko *KustomizeEngine) {
		ko.ResourcePath = path
	}
}

// AdditionalKustomizationPatcher allows you to append KustomizationPatcher objects
// to the patch pipeline.
func AdditionalKustomizationPatcher(op ...KustomizationPatcher) KustomizeOption {
	return func(ko *KustomizeEngine) {
		ko.Patcher = append(ko.Patcher, op...)
	}
}

// NewKustomizeEngine returns a KustomizeEngine object. rootPath should
// point to the folder where your base kustomization.yaml resides and patcher
// is the chain of KustomizationPatcher that makes modifications of Kustomization
// object.
func NewKustomizeEngine(opt ...KustomizeOption) *KustomizeEngine {
	ko := &KustomizeEngine{
		ResourcePath: defaultRootPath,
		Patcher: KustomizationPatcherChain{
			NewNamePrefixer(),
			NewLabelPropagator(),
			NewVarReferenceFiller(),
		},
	}

	for _, f := range opt {
		f(ko)
	}

	return ko
}

type KustomizeEngine struct {
	// ResourcePath is the folder that the base resources reside in the
	// filesystem. It should be given as absolute path.
	ResourcePath string

	// Patcher contains the modifications that you'd like to make to
	// the overlay Kustomization object before calling kustomize.
	Patcher KustomizationPatcherChain
}

func (o *KustomizeEngine) Run(cr resource.ParentResource) ([]resource.ChildResource, error) {
	k := &kustomizeapi.Kustomization{}
	tmpl, err := ioutil.ReadFile(filepath.Join(o.ResourcePath, templateFileName))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if !os.IsNotExist(err) {
		if err := yaml.Unmarshal(tmpl, k); err != nil {
			return nil, err
		}
	}
	if err := o.Patcher.Patch(cr, k); err != nil {
		return nil, errors.Wrap(err, errPatch)
	}
	dir, err := o.prepareOverlay(cr, k)
	defer os.RemoveAll(dir)
	if err != nil {
		return nil, errors.Wrap(err, errOverlayPreparation)
	}
	kustomizer := krusty.MakeKustomizer(filesys.MakeFsOnDisk(), krusty.MakeDefaultOptions())
	resMap, err := kustomizer.Run(dir)
	if err != nil {
		return nil, errors.Wrap(err, errKustomizeCall)
	}
	var objects []resource.ChildResource
	for _, res := range resMap.Resources() {
		u := &unstructured.Unstructured{}
		// NOTE(muvaf): This is magic.
		u.SetUnstructuredContent(res.Map())

		// NOTE(muvaf): ParentResource is written to kustomization directory
		// only to be used for value retrieval. We remove it from the render
		// results here.
		if u.GroupVersionKind() == cr.GetObjectKind().GroupVersionKind() {
			continue
		}

		objects = append(objects, u)
	}

	return objects, nil
}

func (o *KustomizeEngine) prepareOverlay(cr resource.ParentResource, k *kustomizeapi.Kustomization) (string, error) {
	// NOTE(muvaf): Kustomize does not work with symlinked paths, so, we're
	// using their temp directory generation function that handles this instead
	// of Golang's.
	tempConfirmedDir, err := filesys.NewTmpConfirmedDir()
	if err != nil {
		return "", err
	}
	tempDir := string(tempConfirmedDir)

	crYAML, err := yaml.Marshal(cr)
	if err != nil {
		return "", err
	}
	if err := ioutil.WriteFile(filepath.Join(tempDir, temporaryCRFileName), crYAML, os.ModePerm); err != nil {
		return "", err
	}

	// NOTE(muvaf): Kustomize doesn't work with absolute paths, all paths have
	// to be relative to the root path of the folder where kustomize points to,
	// which is the temporary directory we created.
	absPath, err := filepath.Abs(o.ResourcePath)
	if err != nil {
		return "", err
	}
	relPath, err := filepath.Rel(tempDir, absPath)
	if err != nil {
		return "", err
	}
	k.Resources = append(k.Resources, []string{relPath, temporaryCRFileName}...)
	yamlData, err := yaml.Marshal(k)
	if err != nil {
		return "", err
	}
	if err := ioutil.WriteFile(filepath.Join(tempDir, kustomizationFileName), yamlData, os.ModePerm); err != nil {
		return "", err
	}
	return tempDir, nil
}
