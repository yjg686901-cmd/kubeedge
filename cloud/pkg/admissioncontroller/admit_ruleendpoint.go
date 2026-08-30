package admissioncontroller

import (
	"fmt"
	"net/http"
	"strconv"

	admissionv1 "k8s.io/api/admission/v1"

	rulesv1 "github.com/kubeedge/api/apis/rules/v1"
)

func admitRuleEndpoint(review admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	return validationHandler[rulesv1.RuleEndpoint]{
		newObject: func() *rulesv1.RuleEndpoint { return &rulesv1.RuleEndpoint{} },
		validations: map[admissionv1.Operation]validationFunc[rulesv1.RuleEndpoint]{
			admissionv1.Create:  func(endpoint, _ *rulesv1.RuleEndpoint) error { return validateRuleEndpoint(endpoint) },
			admissionv1.Delete:  nil,
			admissionv1.Connect: nil,
		},
	}.admit(review)
}

func validateRuleEndpoint(ruleEndpoint *rulesv1.RuleEndpoint) error {
	switch ruleEndpoint.Spec.RuleEndpointType {
	case rulesv1.RuleEndpointTypeServiceBus:
		portStr, exist := ruleEndpoint.Spec.Properties["service_port"]
		if !exist {
			return fmt.Errorf("\"service_port\" property missed in property when ruleEndpoint is \"servicebus\"")
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("port should be integer")
		}
		if port < 1 || port > 65535 {
			return fmt.Errorf("port must be in range 1-65535")
		}
	}
	return nil
}

func serveRuleEndpoint(w http.ResponseWriter, r *http.Request) {
	serve(w, r, admitRuleEndpoint)
}
