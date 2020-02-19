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

package resource

import (
	"encoding/json"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
)

// TODO(muvaf): this is kind of hacky. We need to revise the logic to get rid of
// json Marsha/Unmarshal stuff.

// GetCondition returns the condition for the given ConditionType if exists,
// otherwise returns nil
func GetCondition(cr interface{ UnstructuredContent() map[string]interface{} }, ct v1alpha1.ConditionType) (v1alpha1.Condition, error) {
	fetchedConditions, exists, err := unstructured.NestedFieldCopy(cr.UnstructuredContent(), "status", "conditions")
	if err != nil {
		return v1alpha1.Condition{}, err
	}
	if !exists {
		return v1alpha1.Condition{Type: ct, Status: v1.ConditionUnknown}, nil
	}
	conditionsJSON, err := json.Marshal(fetchedConditions)
	if err != nil {
		return v1alpha1.Condition{}, err
	}
	conditions := []v1alpha1.Condition{}
	if err := json.Unmarshal(conditionsJSON, &conditions); err != nil {
		return v1alpha1.Condition{}, err
	}
	for _, c := range conditions {
		if c.Type == ct {
			return c, nil
		}
	}
	return v1alpha1.Condition{Type: ct, Status: v1.ConditionUnknown}, err
}

// SetConditions sets the supplied conditions, replacing any existing conditions
// of the same type. This is a no-op if all supplied conditions are identical,
// ignoring the last transition time, to those already set.
func SetConditions(cr interface{ UnstructuredContent() map[string]interface{} }, c ...v1alpha1.Condition) error {
	conditions := []v1alpha1.Condition{}
	fetched, exists, err := unstructured.NestedFieldCopy(cr.UnstructuredContent(), "status", "conditions")
	if err != nil {
		return err
	}
	if exists {
		statusJSON, err := json.Marshal(fetched)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(statusJSON, &conditions); err != nil {
			return err
		}
	}

	for _, newC := range c {
		exists := false
		for i, existing := range conditions {
			if existing.Type != newC.Type {
				continue
			}

			if existing.Equal(newC) {
				exists = true
				continue
			}

			conditions[i] = newC
			exists = true
		}
		if !exists {
			conditions = append(conditions, newC)
		}
	}
	resultJSON, err := json.Marshal(conditions)
	if err != nil {
		return err
	}
	finalForm := []interface{}{}
	if err := json.Unmarshal(resultJSON, &finalForm); err != nil {
		return err
	}
	return unstructured.SetNestedSlice(cr.UnstructuredContent(), finalForm, "status", "conditions")
}
