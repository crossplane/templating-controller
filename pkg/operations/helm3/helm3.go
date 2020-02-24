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
	"fmt"
	"io"

	"github.com/pkg/errors"

	"github.com/crossplane/crossplane-runtime/pkg/logging"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/crossplane/templating-controller/pkg/resource"
)

const (
	defaultRootPath = "resources"

	errEmptyParentResource = "parent resource is empty"
	errSpecCast            = "parent resource spec could not be casted into a map[string]interface{}"
	errParse               = "could not parse the generated YAMLs"
)

// WithResourcePath returns an Option that changes the resource path of the Engine.
func WithResourcePath(path string) Option {
	return func(h *Engine) {
		h.ResourcePath = path
	}
}

// WithLogger returns an Option that changes the logger of the Engine.
func WithLogger(l logging.Logger) Option {
	return func(h *Engine) {
		// NOTE(muvaf): Even though l.Debug seems to satisfy action.DebugLog interface,
		// they are completely different given that former user the first argument
		// as context while the latter uses it as format string.
		h.log = func(format string, v ...interface{}) {
			l.Debug(fmt.Sprintf(format, v...))
		}
	}
}

// NewHelm3Engine returns a new Helm3 Engine to be used as resource.TemplatingEngine.
func NewHelm3Engine(o ...Option) resource.TemplatingEngine {
	h := &Engine{
		ResourcePath: defaultRootPath,
	}
	for _, f := range o {
		f(h)
	}
	return h
}

// Engine is used to do the templating operation via Helm3.
type Engine struct {
	// ResourcePath is the folder that the base resources reside in the
	// filesystem. It should be given as absolute path.
	ResourcePath string

	// log is used by helm library to log the debugging level logs.
	log action.DebugLog
}

// Run returns the result of the templating operation.
func (h *Engine) Run(cr resource.ParentResource) ([]resource.ChildResource, error) {
	chart, err := loader.Load(h.ResourcePath)
	if err != nil {
		return nil, err
	}
	config := action.Configuration{}
	// NOTE(muvaf): RESTGetter is skipped because we don't need to talk with cluster.
	// namespace is skipped because we use "memory" as storage rather than actual
	// ConfigMap or Secret objects.
	if err := config.Init(nil, "", "memory", h.log); err != nil {
		return nil, err
	}

	if cr.UnstructuredContent() == nil {
		return nil, errors.New(errEmptyParentResource)
	}
	values := map[string]interface{}{}
	valuesMap, exists := cr.UnstructuredContent()["spec"]
	if exists {
		valuesCasted, ok := valuesMap.(map[string]interface{})
		if !ok {
			return nil, errors.New(errSpecCast)
		}
		values = valuesCasted
	}

	i := action.NewInstall(&config)
	i.ReleaseName = cr.GetName()

	// NOTE(muvaf): These settings are same with `helm template`'s call settings.
	i.DryRun = true
	i.Replace = true
	i.ClientOnly = true

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
			return nil, errors.Wrap(err, errParse)
		}
		if err == io.EOF {
			break
		}
		result = append(result, u)
	}
	return result, nil
}
