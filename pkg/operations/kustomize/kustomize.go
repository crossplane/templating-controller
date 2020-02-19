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

	"github.com/crossplane/templating-controller/pkg/resource"
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

// WithResourcePath allows you to specify a kustomization folder other than default.
func WithResourcePath(path string) Option {
	return func(ko *Engine) {
		ko.ResourcePath = path
	}
}

// AdditionalKustomizationPatcher allows you to append Patcher objects
// to the patch pipeline.
func AdditionalKustomizationPatcher(op ...Patcher) Option {
	return func(ko *Engine) {
		ko.Patchers = append(ko.Patchers, op...)
	}
}

// AdditionalOverlayGenerator allows you to append OverlayGenerator objects
// to the generation pipeline.
func AdditionalOverlayGenerator(op ...OverlayGenerator) Option {
	return func(ko *Engine) {
		ko.OverlayGenerators = append(ko.OverlayGenerators, op...)
	}
}

// NewKustomizeEngine returns a Engine object. rootPath should
// point to the folder where your base kustomization.yaml resides and patcher
// is the chain of Patcher that makes modifications of Kustomization
// object.
func NewKustomizeEngine(k *kustomizeapi.Kustomization, opt ...Option) *Engine {
	ko := &Engine{
		ResourcePath:  defaultRootPath,
		Kustomization: k,
		Patchers: KustomizationPatcherChain{
			// TODO(muvaf): think how this should work if name prefix is already
			// given.
			NewNamePrefixer(),
		},
	}

	for _, f := range opt {
		f(ko)
	}

	return ko
}

type Engine struct {
	// ResourcePath is the folder that the base resources reside in the
	// filesystem. It should be given as absolute path.
	ResourcePath string

	// Kustomization is the content of kustomization.yaml file that contains
	// Kustomize config.
	Kustomization *kustomizeapi.Kustomization

	// Patchers contains the modifications that you'd like to make to
	// the overlay Kustomization object before calling kustomize.
	Patchers KustomizationPatcherChain

	// OverlayGenerators contains the overlay generators that will be added
	// to the file system alongside kustomization.yaml
	OverlayGenerators OverlayGeneratorChain
}

func (o *Engine) Run(cr resource.ParentResource) ([]resource.ChildResource, error) {
	if err := o.Patchers.Patch(cr, o.Kustomization); err != nil {
		return nil, errors.Wrap(err, errPatch)
	}
	extraFiles, err := o.OverlayGenerators.Generate(cr, o.Kustomization)
	if err != nil {
		return nil, errors.Wrap(err, "overlay generator failed")
	}
	dir, err := o.prepareOverlay(o.Kustomization, extraFiles)
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
		objects = append(objects, u)
	}
	return objects, nil
}

func (o *Engine) prepareOverlay(k *kustomizeapi.Kustomization, extraFiles []OverlayFile) (string, error) {
	// NOTE(muvaf): Kustomize does not work with symlinked paths, so, we're
	// using their temp directory generation function that handles this instead
	// of Golang's.
	tempConfirmedDir, err := filesys.NewTmpConfirmedDir()
	if err != nil {
		return "", err
	}
	tempDir := string(tempConfirmedDir)

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
	k.Resources = appendIfNotExists(k.Resources, relPath)
	yamlData, err := yaml.Marshal(k)
	if err != nil {
		return "", err
	}
	if err := ioutil.WriteFile(filepath.Join(tempDir, kustomizationFileName), yamlData, os.ModePerm); err != nil {
		return "", err
	}
	for _, file := range extraFiles {
		if err := ioutil.WriteFile(filepath.Join(tempDir, file.Name), file.Data, os.ModePerm); err != nil {
			return "", err
		}
	}
	return tempDir, nil
}

// todo: temporary.
func appendIfNotExists(arr []string, obj string) []string {
	for _, e := range arr {
		if e == obj {
			return arr
		}
	}
	return append(arr, obj)
}
