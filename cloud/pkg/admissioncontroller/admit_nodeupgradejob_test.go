/*
Copyright 2024 The KubeEdge Authors.

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
	"testing"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kubeedge/api/apis/operations/v1alpha1"
)

func TestAdmitNodeUpgradeJob(t *testing.T) {
	assert := assert.New(t)

	testCases := []struct {
		name            string
		operation       admissionv1.Operation
		upgrade         *v1alpha1.NodeUpgradeJob
		oldUpgrade      *v1alpha1.NodeUpgradeJob
		expectedAllowed bool
		expectedError   string
	}{
		{
			name:      "Valid Create",
			operation: admissionv1.Create,
			upgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version:   "v1.0.0",
					NodeNames: []string{"node1", "node2"},
				},
			},
			expectedAllowed: true,
		},
		{
			name:      "Invalid Version",
			operation: admissionv1.Create,
			upgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version:   "1.0.0",
					NodeNames: []string{"node1"},
				},
			},
			expectedAllowed: false,
			expectedError:   "invalid version 1.0.0",
		},
		{
			name:      "Invalid Semver",
			operation: admissionv1.Create,
			upgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version:   "v1.0",
					NodeNames: []string{"node1"},
				},
			},
			expectedAllowed: false,
			expectedError:   "invalid version v1.0",
		},
		{
			name:      "Invalid image",
			operation: admissionv1.Create,
			upgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version: "v1.0.0",
					Image:   "invalid-image",
				},
			},
			expectedAllowed: false,
			expectedError:   "invalid image repo invalid-image",
		},
		{
			name:      "No NodeNames and LabelSelector",
			operation: admissionv1.Create,
			upgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version: "v1.0.0",
				},
			},
			expectedAllowed: false,
			expectedError:   "both NodeNames and LabelSelector are NOT specified",
		},
		{
			name:      "Both NodeNames and LabelSelector",
			operation: admissionv1.Create,
			upgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version:       "v1.0.0",
					NodeNames:     []string{"node1"},
					LabelSelector: &metav1.LabelSelector{},
				},
			},
			expectedAllowed: false,
			expectedError:   "both NodeNames and LabelSelector are specified",
		},
		{
			name:      "Valid Update",
			operation: admissionv1.Update,
			upgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version:   "v1.0.0",
					NodeNames: []string{"node1", "node2"},
				},
			},
			oldUpgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version:   "v1.0.0",
					NodeNames: []string{"node1", "node2"},
				},
			},
			expectedAllowed: true,
		},
		{
			name:      "Invalid Update - Spec Change",
			operation: admissionv1.Update,
			upgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version:   "v1.0.1",
					NodeNames: []string{"node1", "node2"},
				},
			},
			oldUpgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version:   "v1.0.0",
					NodeNames: []string{"node1", "node2"},
				},
			},
			expectedAllowed: false,
			expectedError:   "spec fields are not allowed to update once it's created",
		},
		{
			name:            "Valid Delete",
			operation:       admissionv1.Delete,
			expectedAllowed: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			review := admissionv1.AdmissionReview{
				Request: &admissionv1.AdmissionRequest{
					Operation: tc.operation,
				},
			}

			if tc.upgrade != nil {
				raw, _ := json.Marshal(tc.upgrade)
				review.Request.Object = runtime.RawExtension{Raw: raw}
			}

			if tc.oldUpgrade != nil {
				raw, _ := json.Marshal(tc.oldUpgrade)
				review.Request.OldObject = runtime.RawExtension{Raw: raw}
			}

			response := admitNodeUpgradeJob(review)

			assert.Equal(tc.expectedAllowed, response.Allowed)
			if tc.expectedError != "" {
				assert.Contains(response.Result.Message, tc.expectedError)
			} else {
				assert.Nil(response.Result)
			}
		})
	}
}

func TestValidateNodeUpgradeJob(t *testing.T) {
	assert := assert.New(t)

	testCases := []struct {
		name        string
		upgrade     *v1alpha1.NodeUpgradeJob
		expectedErr string
	}{
		{
			name: "Valid upgrade job",
			upgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version:   "v1.0.0",
					NodeNames: []string{"node1", "node2"},
				},
			},
			expectedErr: "",
		},
		{
			name: "Invalid version",
			upgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version:   "1.0.0",
					NodeNames: []string{"node1"},
				},
			},
			expectedErr: "invalid version 1.0.0",
		},
		{
			name: "Invalid image",
			upgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version: "v1.0.0",
					Image:   "invalid-image",
				},
			},
			expectedErr: "invalid image repo invalid-image",
		},
		{
			name: "Invalid version (not semver compatible)",
			upgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version:   "v1.0",
					NodeNames: []string{"node1"},
				},
			},
			expectedErr: "invalid version v1.0",
		},
		{
			name: "Missing both NodeNames and LabelSelector",
			upgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version: "v1.0.0",
				},
			},
			expectedErr: "both NodeNames and LabelSelector are NOT specified",
		},
		{
			name: "Both NodeNames and LabelSelector specified",
			upgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version:       "v1.0.0",
					NodeNames:     []string{"node1"},
					LabelSelector: &metav1.LabelSelector{},
				},
			},
			expectedErr: "both NodeNames and LabelSelector are specified",
		},
		{
			name: "Valid upgrade job",
			upgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version:       "v1.0.0",
					LabelSelector: &metav1.LabelSelector{},
				},
			},
			expectedErr: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNodeUpgradeJob(tc.upgrade)
			if tc.expectedErr == "" {
				assert.NoError(err)
			} else {
				assert.Error(err)
				assert.Contains(err.Error(), tc.expectedErr)
			}
		})
	}
}

func TestAdmissionResponse(t *testing.T) {
	assert := assert.New(t)

	testCases := []struct {
		name            string
		inputError      error
		expectedAllowed bool
		expectedMessage string
	}{
		{
			name:            "No error",
			inputError:      nil,
			expectedAllowed: true,
			expectedMessage: "",
		},
		{
			name:            "With error",
			inputError:      errors.New("validation failed"),
			expectedAllowed: false,
			expectedMessage: "validation failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			response := admissionResponse(tc.inputError)

			assert.NotNil(response)
			assert.Equal(tc.expectedAllowed, response.Allowed)

			if tc.inputError != nil {
				assert.NotNil(response.Result)
				assert.Equal(tc.expectedMessage, response.Result.Message)
			} else {
				assert.Nil(response.Result)
			}
		})
	}
}

func TestMutatingNodeUpgradeJob(t *testing.T) {
	cases := []struct {
		name       string
		raw        []byte
		wantPaths  []string
		wantValues map[string]uint32
	}{
		{
			name:      "adds both defaults when fields are absent",
			raw:       []byte(`{"spec":{}}`),
			wantPaths: []string{"/spec/concurrency", "/spec/timeoutSeconds"},
			wantValues: map[string]uint32{
				"concurrency": 1, "timeoutSeconds": 300,
			},
		},
		{
			name:      "preserves explicit zero concurrency",
			raw:       []byte(`{"spec":{"concurrency":0}}`),
			wantPaths: []string{"/spec/timeoutSeconds"},
			wantValues: map[string]uint32{
				"concurrency": 0, "timeoutSeconds": 300,
			},
		},
		{
			name:      "preserves explicit nonzero concurrency",
			raw:       []byte(`{"spec":{"concurrency":5}}`),
			wantPaths: []string{"/spec/timeoutSeconds"},
			wantValues: map[string]uint32{
				"concurrency": 5, "timeoutSeconds": 300,
			},
		},
		{
			name:      "preserves explicit timeoutSeconds",
			raw:       []byte(`{"spec":{"timeoutSeconds":900}}`),
			wantPaths: []string{"/spec/concurrency"},
			wantValues: map[string]uint32{
				"concurrency": 1, "timeoutSeconds": 900,
			},
		},
		{
			name:      "adds only missing timeoutSeconds",
			raw:       []byte(`{"spec":{"concurrency":2}}`),
			wantPaths: []string{"/spec/timeoutSeconds"},
			wantValues: map[string]uint32{
				"concurrency": 2, "timeoutSeconds": 300,
			},
		},
		{
			name: "returns empty patch when both fields are present",
			raw:  []byte(`{"spec":{"concurrency":0,"timeoutSeconds":0}}`),
			wantValues: map[string]uint32{
				"concurrency": 0, "timeoutSeconds": 0,
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			response := mutatingNodeUpgradeJob(admissionv1.AdmissionReview{Request: &admissionv1.AdmissionRequest{
				Object: runtime.RawExtension{Raw: tt.raw},
			}})
			if !response.Allowed {
				t.Fatalf("response was rejected: %#v", response.Result)
			}
			if len(tt.wantPaths) == 0 {
				if response.Patch != nil || response.PatchType != nil {
					t.Fatalf("unexpected patch: %s", response.Patch)
				}
				return
			}

			var patch []patchValue
			if err := json.Unmarshal(response.Patch, &patch); err != nil {
				t.Fatal(err)
			}
			if len(patch) != len(tt.wantPaths) {
				t.Fatalf("patch = %#v, want paths %#v", patch, tt.wantPaths)
			}
			for i, path := range tt.wantPaths {
				if patch[i].Op != "add" || patch[i].Path != path {
					t.Fatalf("patch = %#v, want add %s", patch[i], path)
				}
			}

			mutatedRaw := applyNodeUpgradeJobPatch(t, tt.raw, response.Patch)
			var mutated struct {
				Spec map[string]uint32 `json:"spec"`
			}
			if err := json.Unmarshal(mutatedRaw, &mutated); err != nil {
				t.Fatal(err)
			}
			if !assert.Equal(t, tt.wantValues, mutated.Spec) {
				t.Fatalf("mutated object = %s", mutatedRaw)
			}
		})
	}
}

func TestMutatingNodeUpgradeJobIsIdempotent(t *testing.T) {
	raw := []byte(`{"spec":{}}`)
	firstResponse := mutatingNodeUpgradeJob(admissionv1.AdmissionReview{Request: &admissionv1.AdmissionRequest{
		Object: runtime.RawExtension{Raw: raw},
	}})
	if !firstResponse.Allowed || firstResponse.Patch == nil {
		t.Fatalf("first response = %#v", firstResponse)
	}

	mutatedRaw := applyNodeUpgradeJobPatch(t, raw, firstResponse.Patch)
	secondResponse := mutatingNodeUpgradeJob(admissionv1.AdmissionReview{Request: &admissionv1.AdmissionRequest{
		Object: runtime.RawExtension{Raw: mutatedRaw},
	}})
	if !secondResponse.Allowed {
		t.Fatalf("second response was rejected: %#v", secondResponse.Result)
	}
	if secondResponse.Patch != nil || secondResponse.PatchType != nil {
		t.Fatalf("second mutation produced patch: %s", secondResponse.Patch)
	}
}

func applyNodeUpgradeJobPatch(t *testing.T, raw, rawPatch []byte) []byte {
	t.Helper()
	patch, err := jsonpatch.DecodePatch(rawPatch)
	if err != nil {
		t.Fatal(err)
	}
	mutated, err := patch.Apply(raw)
	if err != nil {
		t.Fatal(err)
	}
	return mutated
}

func TestValidateNodeUpgradeJobAllowsOptionalAndValidImage(t *testing.T) {
	cases := []struct {
		name    string
		upgrade *v1alpha1.NodeUpgradeJob
	}{
		{
			name: "empty image is allowed",
			upgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version:   "v1.0.0",
					NodeNames: []string{"node1"},
				},
			},
		},
		{
			name: "valid image repo is allowed",
			upgrade: &v1alpha1.NodeUpgradeJob{
				Spec: v1alpha1.NodeUpgradeJobSpec{
					Version:   "v1.0.0",
					Image:     "kubeedge/installation-package:v1.23.1",
					NodeNames: []string{"node1"},
				},
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateNodeUpgradeJob(tt.upgrade); err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
