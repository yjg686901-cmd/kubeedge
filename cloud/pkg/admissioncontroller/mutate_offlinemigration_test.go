package admissioncontroller

import (
	"encoding/json"
	"reflect"
	"testing"

	jsonpatch "github.com/evanphx/json-patch"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestMutateOfflineMigration(t *testing.T) {
	otherToleration := corev1.Toleration{
		Key:      "example.com/maintenance",
		Operator: corev1.TolerationOpEqual,
		Value:    "true",
		Effect:   corev1.TaintEffectNoSchedule,
	}
	customUnreachableToleration := corev1.Toleration{
		Key:               corev1.TaintNodeUnreachable,
		Operator:          corev1.TolerationOpExists,
		Effect:            corev1.TaintEffectNoExecute,
		TolerationSeconds: int64Ptr(120),
	}

	cases := []struct {
		name          string
		pod           corev1.Pod
		rawPod        []byte
		wantPatchPath string
		wantTolerated []corev1.Toleration
		wantNoPatch   bool
	}{
		{
			name:          "adds default toleration when tolerations are absent",
			pod:           corev1.Pod{},
			wantPatchPath: "/spec/tolerations",
			wantTolerated: []corev1.Toleration{defaultUnreachableToleration()},
		},
		{
			name:          "adds default toleration when tolerations are empty",
			rawPod:        []byte(`{"spec":{"tolerations":[]}}`),
			wantPatchPath: "/spec/tolerations",
			wantTolerated: []corev1.Toleration{defaultUnreachableToleration()},
		},
		{
			name:          "appends default toleration without replacing existing tolerations",
			pod:           corev1.Pod{Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{otherToleration}}},
			wantPatchPath: "/spec/tolerations/-",
			wantTolerated: []corev1.Toleration{otherToleration, defaultUnreachableToleration()},
		},
		{
			name:          "preserves custom unreachable toleration and adds autonomy toleration",
			pod:           corev1.Pod{Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{otherToleration, customUnreachableToleration}}},
			wantPatchPath: "/spec/tolerations/-",
			wantTolerated: []corev1.Toleration{otherToleration, customUnreachableToleration, defaultUnreachableToleration()},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rawPod := tt.rawPod
			if rawPod == nil {
				var err error
				rawPod, err = json.Marshal(tt.pod)
				if err != nil {
					t.Fatal(err)
				}
			}

			response := mutateOfflineMigration(admissionv1.AdmissionReview{Request: &admissionv1.AdmissionRequest{
				Object: runtime.RawExtension{Raw: rawPod},
			}})
			if !response.Allowed {
				t.Fatalf("response was rejected: %#v", response.Result)
			}
			if tt.wantNoPatch {
				if response.Patch != nil || response.PatchType != nil {
					t.Fatalf("unexpected patch: %s", response.Patch)
				}
				return
			}

			var patch []patchMapValue
			if err := json.Unmarshal(response.Patch, &patch); err != nil {
				t.Fatal(err)
			}
			if len(patch) != 1 || patch[0].Op != "add" || patch[0].Path != tt.wantPatchPath {
				t.Fatalf("patch = %#v", patch)
			}

			mutatedRawPod := applyOfflineMigrationPatch(t, rawPod, response.Patch)
			mutatedPod := corev1.Pod{}
			if err := json.Unmarshal(mutatedRawPod, &mutatedPod); err != nil {
				t.Fatal(err)
			}
			if !tolerationsEqual(mutatedPod.Spec.Tolerations, tt.wantTolerated) {
				t.Fatalf("tolerations = %#v, want %#v", mutatedPod.Spec.Tolerations, tt.wantTolerated)
			}
		})
	}
}

func TestMutateOfflineMigrationIsIdempotent(t *testing.T) {
	rawPod := []byte(`{"spec":{"tolerations":[{"key":"example.com/maintenance","operator":"Equal","value":"true","effect":"NoSchedule"}]}}`)
	firstResponse := mutateOfflineMigration(admissionv1.AdmissionReview{Request: &admissionv1.AdmissionRequest{
		Object: runtime.RawExtension{Raw: rawPod},
	}})
	if !firstResponse.Allowed || firstResponse.Patch == nil {
		t.Fatalf("first response = %#v", firstResponse)
	}

	mutatedRawPod := applyOfflineMigrationPatch(t, rawPod, firstResponse.Patch)
	secondResponse := mutateOfflineMigration(admissionv1.AdmissionReview{Request: &admissionv1.AdmissionRequest{
		Object: runtime.RawExtension{Raw: mutatedRawPod},
	}})
	if !secondResponse.Allowed {
		t.Fatalf("second response was rejected: %#v", secondResponse.Result)
	}
	if secondResponse.Patch != nil || secondResponse.PatchType != nil {
		t.Fatalf("second mutation produced patch: %s", secondResponse.Patch)
	}
}

func applyOfflineMigrationPatch(t *testing.T, rawPod, rawPatch []byte) []byte {
	t.Helper()
	patch, err := jsonpatch.DecodePatch(rawPatch)
	if err != nil {
		t.Fatal(err)
	}
	mutatedRawPod, err := patch.Apply(rawPod)
	if err != nil {
		t.Fatal(err)
	}
	return mutatedRawPod
}

func defaultUnreachableToleration() corev1.Toleration {
	return corev1.Toleration{
		Key:      corev1.TaintNodeUnreachable,
		Operator: corev1.TolerationOpExists,
	}
}

func tolerationsEqual(got, want []corev1.Toleration) bool {
	return reflect.DeepEqual(got, want)
}

func int64Ptr(value int64) *int64 {
	return &value
}
