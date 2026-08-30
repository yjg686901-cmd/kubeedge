package admissioncontroller

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	devicesv1beta1 "github.com/kubeedge/api/apis/devices/v1beta1"
)

type adapterTestObject struct {
	Value string `json:"value"`
}

func TestValidationHandlerOperationAndOldObjectContract(t *testing.T) {
	handler := validationHandler[adapterTestObject]{
		newObject: func() *adapterTestObject { return &adapterTestObject{} },
		validations: map[admissionv1.Operation]validationFunc[adapterTestObject]{
			admissionv1.Create: func(newObject, _ *adapterTestObject) error {
				if newObject.Value == "invalid" {
					return errors.New("invalid object")
				}
				return nil
			},
			admissionv1.Update: func(newObject, oldObject *adapterTestObject) error {
				if oldObject.Value != newObject.Value {
					return errors.New("value is immutable")
				}
				return nil
			},
			admissionv1.Delete: nil,
		},
		requiresOldObject: map[admissionv1.Operation]bool{admissionv1.Update: true},
	}

	makeReview := func(operation admissionv1.Operation, newObject, oldObject *adapterTestObject) admissionv1.AdmissionReview {
		review := admissionv1.AdmissionReview{Request: &admissionv1.AdmissionRequest{Operation: operation}}
		if newObject != nil {
			review.Request.Object.Raw, _ = json.Marshal(newObject)
		}
		if oldObject != nil {
			review.Request.OldObject.Raw, _ = json.Marshal(oldObject)
		}
		return review
	}

	cases := []struct {
		name    string
		review  admissionv1.AdmissionReview
		allowed bool
		message string
	}{
		{
			name:    "create valid object",
			review:  makeReview(admissionv1.Create, &adapterTestObject{Value: "valid"}, nil),
			allowed: true,
		},
		{
			name:    "create invalid object",
			review:  makeReview(admissionv1.Create, &adapterTestObject{Value: "invalid"}, nil),
			allowed: false,
			message: "invalid object",
		},
		{
			name:    "update uses old object",
			review:  makeReview(admissionv1.Update, &adapterTestObject{Value: "new"}, &adapterTestObject{Value: "old"}),
			allowed: false,
			message: "value is immutable",
		},
		{
			name:    "delete is allowed without object decoding",
			review:  makeReview(admissionv1.Delete, nil, nil),
			allowed: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			response := handler.admit(tt.review)
			if response.Allowed != tt.allowed {
				t.Fatalf("Allowed = %t, want %t", response.Allowed, tt.allowed)
			}
			if tt.message == "" {
				if response.Result != nil {
					t.Fatalf("unexpected rejection message: %q", response.Result.Message)
				}
				return
			}
			if response.Result == nil || response.Result.Message != tt.message {
				t.Fatalf("message = %#v, want %q", response.Result, tt.message)
			}
		})
	}
}

func TestServePreservesUIDAndValidationMessage(t *testing.T) {
	device := devicesv1beta1.Device{Spec: devicesv1beta1.DeviceSpec{Properties: []devicesv1beta1.DeviceProperty{{Name: "duplicate"}, {Name: "duplicate"}}}}
	raw, err := json.Marshal(device)
	if err != nil {
		t.Fatal(err)
	}

	review := admissionv1.AdmissionReview{Request: &admissionv1.AdmissionRequest{
		UID:       types.UID("validation-request-uid"),
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: raw},
	}}
	body, err := json.Marshal(review)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/devices", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	serve(recorder, request, admitDevice)

	responseReview := admissionv1.AdmissionReview{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &responseReview); err != nil {
		t.Fatal(err)
	}
	if responseReview.Response == nil {
		t.Fatal("response is missing")
	}
	if responseReview.Response.UID != review.Request.UID {
		t.Fatalf("UID = %q, want %q", responseReview.Response.UID, review.Request.UID)
	}
	if responseReview.Response.Allowed {
		t.Fatal("invalid device was allowed")
	}
	if responseReview.Response.Result == nil || responseReview.Response.Result.Message != "property names must be unique." {
		t.Fatalf("message = %#v", responseReview.Response.Result)
	}
}
