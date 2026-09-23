package admissioncontroller

import (
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
)

type patchMapValue struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

func mutateOfflineMigration(review admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	return mutationHandler[corev1.Pod, patchMapValue]{
		newObject: func() *corev1.Pod { return &corev1.Pod{} },
		mutation: func(pod *corev1.Pod) ([]patchMapValue, error) {
			return generatePatch(pod.Spec.Tolerations), nil
		},
	}.admit(review)
}

func generatePatch(tolerations []corev1.Toleration) []patchMapValue {
	for _, toleration := range tolerations {
		// The API server adds a 300-second NoExecute toleration before calling
		// mutating webhooks.  That temporary default must not suppress the
		// permanent autonomy toleration.  Treat only the exact mutation target
		// as already present, which also keeps repeated admission idempotent.
		if toleration.Key == corev1.TaintNodeUnreachable &&
			toleration.Operator == corev1.TolerationOpExists &&
			toleration.Effect == "" && toleration.TolerationSeconds == nil {
			return nil
		}
	}

	defaultToleration := corev1.Toleration{
		Key:      corev1.TaintNodeUnreachable,
		Operator: corev1.TolerationOpExists,
	}
	if len(tolerations) == 0 {
		return []patchMapValue{{
			Op:    "add",
			Path:  "/spec/tolerations",
			Value: []corev1.Toleration{defaultToleration},
		}}
	}

	return []patchMapValue{{
		Op:    "add",
		Path:  "/spec/tolerations/-",
		Value: defaultToleration,
	}}
}

func serveOfflineMigration(w http.ResponseWriter, r *http.Request) {
	serve(w, r, mutateOfflineMigration)
}
