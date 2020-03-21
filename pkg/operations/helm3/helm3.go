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
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/crossplane/crossplane-runtime/pkg/logging"

	"github.com/crossplane/templating-controller/pkg/resource"
)

const (
	defaultRootPath = "resources"

	errSpecCast      = "parent resource spec could not be casted into a map[string]interface{}"
	errParse         = "could not parse the generated YAMLs"
	errHelm3Template = "helm3 template call failed"
)

// WithResourcePath returns an Option that changes the resource path of the engine.
func WithResourcePath(path string) Option {
	return func(e *engine) {
		e.resourcePath = path
	}
}

// WithLogger returns an Option that changes the logger of the engine.
func WithLogger(l logging.Logger) Option {
	return func(e *engine) {
		// NOTE(muvaf): Even though l.Debug seems to satisfy action.DebugLog interface,
		// they are completely different given that former user the first argument
		// as context while the latter uses it as format string.
		e.debugLog = func(format string, v ...interface{}) {
			l.Debug("Helm3", "debuglog", fmt.Sprintf(format, v...))
		}
	}
}

// NewHelm3Engine returns a new Helm3 engine to be used as resource.TemplatingEngine.
func NewHelm3Engine(o ...Option) resource.TemplatingEngine {
	h := &engine{
		resourcePath: defaultRootPath,
	}
	for _, f := range o {
		f(h)
	}
	return h
}

// engine is used to do the templating operation via Helm3.
type engine struct {
	// resourcePath is the folder that the base resources reside in the
	// filesystem. It should be given as absolute path.
	resourcePath string

	// debugLog is used by helm library to debugLog the debugging level logs.
	debugLog action.DebugLog
}

// Run returns the result of the templating operation.
func (e *engine) Run(cr resource.ParentResource) ([]resource.ChildResource, error) {
	values := map[string]interface{}{}
	valuesMap, exists := cr.UnstructuredContent()["spec"]
	if exists {
		valuesCasted, ok := valuesMap.(map[string]interface{})
		if !ok {
			return nil, errors.New(errSpecCast)
		}
		values = valuesCasted
	}
	rawResult, err := e.template(cr.GetName(), values)
	if err != nil {
		return nil, errors.Wrap(err, errHelm3Template)
	}
	resources, err := parse([]byte(rawResult))
	return resources, errors.Wrap(err, errParse)
}

func (e *engine) template(releaseName string, values map[string]interface{}) (string, error) {
	chart, err := loader.Load(e.resourcePath)
	if err != nil {
		return "", err
	}
	config := action.Configuration{}
	// NOTE(muvaf): RESTGetter is skipped because we don't need to talk with cluster.
	// namespace is skipped because we use "memory" as storage rather than actual
	// ConfigMap or Secret objects.
	if err := config.Init(nil, "", "memory", e.debugLog); err != nil {
		return "", err
	}

	i := action.NewInstall(&config)
	i.ReleaseName = releaseName

	// NOTE(muvaf): These settings are same with `helm template`'s call settings.
	i.DryRun = true
	i.Replace = true
	i.ClientOnly = true

	release, err := i.Run(chart, values)
	if err != nil {
		return "", err
	}
	return release.Manifest, nil
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
