package admissioncontroller

import (
	"encoding/json"
	"fmt"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"

	devicesv1beta1 "github.com/kubeedge/api/apis/devices/v1beta1"
	operationsv1alpha1 "github.com/kubeedge/api/apis/operations/v1alpha1"
	rulesv1 "github.com/kubeedge/api/apis/rules/v1"
)

// TestValidatingHandlersMatchLegacyBehavior runs identical AdmissionReviews through
// the migrated handler and a test-only copy of the former HTTP-facing control
// flow.  Keeping the legacy copy in _test.go prevents it from becoming a second
// production implementation while protecting the externally visible contract.
func TestValidatingHandlersMatchLegacyBehavior(t *testing.T) {
	device := devicesv1beta1.Device{Spec: devicesv1beta1.DeviceSpec{Properties: []devicesv1beta1.DeviceProperty{{Name: "same"}, {Name: "same"}}}}
	deviceModel := devicesv1beta1.DeviceModel{Spec: devicesv1beta1.DeviceModelSpec{Properties: []devicesv1beta1.ModelProperty{{Name: "same"}, {Name: "same"}}}}
	endpoint := rulesv1.RuleEndpoint{Spec: rulesv1.RuleEndpointSpec{RuleEndpointType: rulesv1.RuleEndpointTypeServiceBus, Properties: map[string]string{"service_port": "70000"}}}
	upgrade := operationsv1alpha1.NodeUpgradeJob{Spec: operationsv1alpha1.NodeUpgradeJobSpec{Version: "not-a-version", NodeNames: []string{"edge-1"}}}

	cases := []struct {
		name   string
		new    hookFunc
		legacy hookFunc
		review admissionv1.AdmissionReview
	}{
		{"Device/create-invalid", admitDevice, legacyDevice, reviewFor(admissionv1.Create, &device, nil)},
		{"Device/delete", admitDevice, legacyDevice, reviewFor[devicesv1beta1.Device](admissionv1.Delete, nil, nil)},
		{"Device/unsupported", admitDevice, legacyDevice, reviewFor[devicesv1beta1.Device]("unsupported", nil, nil)},
		{"DeviceModel/create-invalid", admitDeviceModel, legacyDeviceModel, reviewFor(admissionv1.Create, &deviceModel, nil)},
		{"DeviceModel/delete", admitDeviceModel, legacyDeviceModel, reviewFor[devicesv1beta1.DeviceModel](admissionv1.Delete, nil, nil)},
		{"Rule/delete", admitRule, legacyRule, reviewFor[rulesv1.Rule](admissionv1.Delete, nil, nil)},
		{"Rule/update-unsupported", admitRule, legacyRule, reviewFor[rulesv1.Rule](admissionv1.Update, nil, nil)},
		{"RuleEndpoint/create-invalid", admitRuleEndpoint, legacyRuleEndpoint, reviewFor(admissionv1.Create, &endpoint, nil)},
		{"RuleEndpoint/delete", admitRuleEndpoint, legacyRuleEndpoint, reviewFor[rulesv1.RuleEndpoint](admissionv1.Delete, nil, nil)},
		{"NodeUpgradeJob/create-invalid", admitNodeUpgradeJob, legacyNodeUpgradeJob, reviewFor(admissionv1.Create, &upgrade, nil)},
		{"NodeUpgradeJob/delete", admitNodeUpgradeJob, legacyNodeUpgradeJob, reviewFor[operationsv1alpha1.NodeUpgradeJob](admissionv1.Delete, nil, nil)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertSameAdmissionResponse(t, tc.legacy(tc.review), tc.new(tc.review))
		})
	}
}

func reviewFor[T any](operation admissionv1.Operation, object, oldObject *T) admissionv1.AdmissionReview {
	review := admissionv1.AdmissionReview{Request: &admissionv1.AdmissionRequest{Operation: operation}}
	if object != nil {
		review.Request.Object.Raw, _ = json.Marshal(object)
	}
	if oldObject != nil {
		review.Request.OldObject.Raw, _ = json.Marshal(oldObject)
	}
	return review
}

func assertSameAdmissionResponse(t *testing.T, want, got *admissionv1.AdmissionResponse) {
	t.Helper()
	if want.Allowed != got.Allowed {
		t.Fatalf("Allowed = %t, want %t", got.Allowed, want.Allowed)
	}
	wantMessage, gotMessage := admissionMessage(want), admissionMessage(got)
	if wantMessage != gotMessage {
		t.Fatalf("message = %q, want %q", gotMessage, wantMessage)
	}
}

func admissionMessage(response *admissionv1.AdmissionResponse) string {
	if response.Result == nil {
		return ""
	}
	return response.Result.Message
}

func legacyDevice(review admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	switch review.Request.Operation {
	case admissionv1.Create, admissionv1.Update:
		device := &devicesv1beta1.Device{}
		if err := json.Unmarshal(review.Request.Object.Raw, device); err != nil {
			return admissionResponse(err)
		}
		return admissionResponse(validateDevice(device))
	case admissionv1.Delete, admissionv1.Connect:
		return admissionResponse(nil)
	default:
		return admissionResponse(fmt.Errorf("Unsupported webhook operation!"))
	}
}

func legacyDeviceModel(review admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	switch review.Request.Operation {
	case admissionv1.Create, admissionv1.Update:
		model := &devicesv1beta1.DeviceModel{}
		if err := json.Unmarshal(review.Request.Object.Raw, model); err != nil {
			return admissionResponse(err)
		}
		return admissionResponse(validateDeviceModel(model))
	case admissionv1.Delete, admissionv1.Connect:
		return admissionResponse(nil)
	default:
		return admissionResponse(fmt.Errorf("Unsupported webhook operation!"))
	}
}

func legacyRule(review admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	switch review.Request.Operation {
	case admissionv1.Create:
		rule := &rulesv1.Rule{}
		if err := json.Unmarshal(review.Request.Object.Raw, rule); err != nil {
			return admissionResponse(err)
		}
		return admissionResponse(validateRule(rule))
	case admissionv1.Delete, admissionv1.Connect:
		return admissionResponse(nil)
	default:
		return admissionResponse(fmt.Errorf("unsupported webhook operation %v", review.Request.Operation))
	}
}

func legacyRuleEndpoint(review admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	switch review.Request.Operation {
	case admissionv1.Create:
		endpoint := &rulesv1.RuleEndpoint{}
		if err := json.Unmarshal(review.Request.Object.Raw, endpoint); err != nil {
			return admissionResponse(err)
		}
		return admissionResponse(validateRuleEndpoint(endpoint))
	case admissionv1.Delete, admissionv1.Connect:
		return admissionResponse(nil)
	default:
		return admissionResponse(fmt.Errorf("unsupported webhook operation %v", review.Request.Operation))
	}
}

func legacyNodeUpgradeJob(review admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	switch review.Request.Operation {
	case admissionv1.Create:
		upgrade := &operationsv1alpha1.NodeUpgradeJob{}
		if err := json.Unmarshal(review.Request.Object.Raw, upgrade); err != nil {
			return admissionResponse(fmt.Errorf("validation failed with error: %v", err))
		}
		return admissionResponse(validateNodeUpgradeJob(upgrade))
	case admissionv1.Update:
		upgrade, oldUpgrade := &operationsv1alpha1.NodeUpgradeJob{}, &operationsv1alpha1.NodeUpgradeJob{}
		if err := json.Unmarshal(review.Request.Object.Raw, upgrade); err != nil {
			return admissionResponse(fmt.Errorf("validation failed with error: %v", err))
		}
		if err := json.Unmarshal(review.Request.OldObject.Raw, oldUpgrade); err != nil {
			return admissionResponse(fmt.Errorf("validation failed with error: %v", err))
		}
		return admissionResponse(validateNodeUpgradeJobUpdate(upgrade, oldUpgrade))
	case admissionv1.Delete:
		return admissionResponse(nil)
	default:
		return admissionResponse(fmt.Errorf("unsupported webhook operation %v", review.Request.Operation))
	}
}
