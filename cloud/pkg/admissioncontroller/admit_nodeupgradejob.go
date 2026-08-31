/*
Copyright 2022 The KubeEdge Authors.

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

package admissioncontroller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"

	admissionv1 "k8s.io/api/admission/v1"

	"github.com/kubeedge/api/apis/operations/v1alpha1"
	"github.com/kubeedge/kubeedge/pkg/util/validation"
)

func serveNodeUpgradeJob(w http.ResponseWriter, r *http.Request) {
	serve(w, r, admitNodeUpgradeJob)
}

func serveMutatingNodeUpgradeJob(w http.ResponseWriter, r *http.Request) {
	serve(w, r, mutatingNodeUpgradeJob)
}

func admitNodeUpgradeJob(review admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	return validationHandler[v1alpha1.NodeUpgradeJob]{
		newObject: func() *v1alpha1.NodeUpgradeJob { return &v1alpha1.NodeUpgradeJob{} },
		validations: map[admissionv1.Operation]validationFunc[v1alpha1.NodeUpgradeJob]{
			admissionv1.Create: func(upgrade, _ *v1alpha1.NodeUpgradeJob) error {
				return validateNodeUpgradeJob(upgrade)
			},
			admissionv1.Update: validateNodeUpgradeJobUpdate,
			admissionv1.Delete: nil,
		},
		requiresOldObject: map[admissionv1.Operation]bool{admissionv1.Update: true},
		decodeError: func(err error) error {
			return fmt.Errorf("validation failed with error: %v", err)
		},
	}.admit(review)
}

func validateNodeUpgradeJobUpdate(newUpgrade, oldUpgrade *v1alpha1.NodeUpgradeJob) error {
	// For update, we don't allow update spec fields once an Upgrade is created.
	if !reflect.DeepEqual(oldUpgrade.Spec, newUpgrade.Spec) {
		return errors.New("spec fields are not allowed to update once it's created")
	}
	return validateNodeUpgradeJob(newUpgrade)
}

func validateNodeUpgradeJob(upgrade *v1alpha1.NodeUpgradeJob) error {
	if !validation.ValidateVersion(upgrade.Spec.Version) {
		return fmt.Errorf("invalid version %s", upgrade.Spec.Version)
	}
	// Image is a optional field.
	if upgrade.Spec.Image != "" && !validation.ValidateImageRepo(upgrade.Spec.Image) {
		return fmt.Errorf("invalid image repo %s", upgrade.Spec.Image)
	}
	// we must specify NodeNames or LabelSelector, and we can only specify only one
	if len(upgrade.Spec.NodeNames) == 0 && upgrade.Spec.LabelSelector == nil {
		return fmt.Errorf("both NodeNames and LabelSelector are NOT specified")
	}
	if len(upgrade.Spec.NodeNames) != 0 && upgrade.Spec.LabelSelector != nil {
		return fmt.Errorf("both NodeNames and LabelSelector are specified")
	}

	return nil
}

func mutatingNodeUpgradeJob(review admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	return mutationHandler[v1alpha1.NodeUpgradeJob, patchValue]{
		newObject: func() *v1alpha1.NodeUpgradeJob { return &v1alpha1.NodeUpgradeJob{} },
		rawMutation: func(_ *v1alpha1.NodeUpgradeJob, rawObject []byte) ([]patchValue, error) {
			return generateNodeUpgradeJobPatch(rawObject)
		},
	}.admit(review)
}

func generateNodeUpgradeJobPatch(rawObject []byte) ([]patchValue, error) {
	var object struct {
		Spec map[string]json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal(rawObject, &object); err != nil {
		return nil, err
	}

	patch := make([]patchValue, 0)

	// Inspect raw JSON so an explicitly supplied concurrency: 0 is preserved.
	if _, exists := object.Spec["concurrency"]; !exists {
		patch = append(patch, patchValue{
			Op:    "add",
			Path:  "/spec/concurrency",
			Value: 1,
		})
	}
	if _, exists := object.Spec["timeoutSeconds"]; !exists {
		patch = append(patch, patchValue{
			Op:    "add",
			Path:  "/spec/timeoutSeconds",
			Value: uint32(300),
		})
	}

	return patch, nil
}

type patchValue struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}
