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
	"bytes"
	"errors"
	"fmt"
	"io"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/crossplaneio/templating-controller/pkg/resource"
)

const (
	defaultRootPath = "resources"
)

func WithResourcePath(path string) Option {
	return func(h *Engine) {
		h.ResourcePath = path
	}
}

func NewHelm3Engine(o ...Option) resource.TemplatingEngine {
	h := &Engine{
		ResourcePath: defaultRootPath,
	}
	for _, f := range o {
		f(h)
	}
	return h
}

type Engine struct {
	// ResourcePath is the folder that the base resources reside in the
	// filesystem. It should be given as absolute path.
	ResourcePath string
}

func (h *Engine) Run(cr resource.ParentResource) ([]resource.ChildResource, error) {
	chart, err := loader.Load(h.ResourcePath)
	if err != nil {
		return nil, err
	}
	config := action.Configuration{}
	// TODO(muvaf): how about cluster-scoped parent resources with no namespace?
	if err := config.Init(nil, cr.GetNamespace(), "memory", func(format string, v ...interface{}) {
		// TODO(muvaf): look for better handling of logging.
		fmt.Printf(format, v)
	}); err != nil {
		return nil, err
	}
	i := action.NewInstall(&config)
	i.DryRun = true
	i.ReleaseName = cr.GetName()
	i.Replace = true // Skip the name check
	i.ClientOnly = true
	// i.APIVersions = chartutil.VersionSet{}
	values := map[string]interface{}{}
	if valuesMap, exists := cr.UnstructuredContent()["spec"]; exists {
		if valuesCasted, ok := valuesMap.(map[string]interface{}); !ok {
			return nil, errors.New("spec could not be parsed into a map")
		} else {
			values = valuesCasted
		}
	}

	release, err := i.Run(chart, values)
	if err != nil {
		return nil, err
	}
	return parse([]byte(release.Manifest))
}

func parse(source []byte) ([]resource.ChildResource, error) {
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(source), 4096)
	var result []resource.ChildResource
	for {
		u := &unstructured.Unstructured{}
		err := dec.Decode(u)
		if err != nil && err != io.EOF {
			return nil, err
		}
		if err == io.EOF {
			break
		}
		result = append(result, u)
	}
	return result, nil
}
