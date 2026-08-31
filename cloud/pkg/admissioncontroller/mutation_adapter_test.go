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
)

type mutationAdapterTestObject struct {
	Value string `json:"value"`
}

type mutationAdapterTestPatch struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

func TestMutationHandler(t *testing.T) {
	makeReview := func(raw []byte) admissionv1.AdmissionReview {
		return admissionv1.AdmissionReview{Request: &admissionv1.AdmissionRequest{
			Object: runtime.RawExtension{Raw: raw},
		}}
	}

	cases := []struct {
		name        string
		handler     mutationHandler[mutationAdapterTestObject, mutationAdapterTestPatch]
		review      admissionv1.AdmissionReview
		allowed     bool
		message     string
		patch       []mutationAdapterTestPatch
		expectPatch bool
	}{
		{
			name: "returns JSON patch response",
			handler: mutationHandler[mutationAdapterTestObject, mutationAdapterTestPatch]{
				newObject: func() *mutationAdapterTestObject { return &mutationAdapterTestObject{} },
				mutation: func(object *mutationAdapterTestObject) ([]mutationAdapterTestPatch, error) {
					return []mutationAdapterTestPatch{{Op: "add", Path: "/value", Value: object.Value}}, nil
				},
			},
			review:      makeReview([]byte(`{"value":"default"}`)),
			allowed:     true,
			expectPatch: true,
			patch:       []mutationAdapterTestPatch{{Op: "add", Path: "/value", Value: "default"}},
		},
		{
			name: "returns empty response when no mutation is needed",
			handler: mutationHandler[mutationAdapterTestObject, mutationAdapterTestPatch]{
				newObject: func() *mutationAdapterTestObject { return &mutationAdapterTestObject{} },
				mutation: func(*mutationAdapterTestObject) ([]mutationAdapterTestPatch, error) {
					return nil, nil
				},
			},
			review:  makeReview([]byte(`{"value":"set"}`)),
			allowed: true,
		},
		{
			name: "rejects decode failure",
			handler: mutationHandler[mutationAdapterTestObject, mutationAdapterTestPatch]{
				newObject:   func() *mutationAdapterTestObject { return &mutationAdapterTestObject{} },
				mutation:    func(*mutationAdapterTestObject) ([]mutationAdapterTestPatch, error) { return nil, nil },
				decodeError: func(error) error { return errors.New("could not decode object") },
			},
			review:  makeReview([]byte(`invalid`)),
			allowed: false,
			message: "could not decode object",
		},
		{
			name: "rejects mutation failure",
			handler: mutationHandler[mutationAdapterTestObject, mutationAdapterTestPatch]{
				newObject: func() *mutationAdapterTestObject { return &mutationAdapterTestObject{} },
				mutation: func(*mutationAdapterTestObject) ([]mutationAdapterTestPatch, error) {
					return nil, errors.New("mutation failed")
				},
				mutationError: func(err error) error { return errors.New("mutate: " + err.Error()) },
			},
			review:  makeReview([]byte(`{"value":"set"}`)),
			allowed: false,
			message: "mutate: mutation failed",
		},
		{
			name: "rejects patch serialization failure",
			handler: mutationHandler[mutationAdapterTestObject, mutationAdapterTestPatch]{
				newObject: func() *mutationAdapterTestObject { return &mutationAdapterTestObject{} },
				mutation: func(*mutationAdapterTestObject) ([]mutationAdapterTestPatch, error) {
					return []mutationAdapterTestPatch{{Op: "add", Path: "/value", Value: make(chan int)}}, nil
				},
			},
			review:  makeReview([]byte(`{"value":"set"}`)),
			allowed: false,
			message: "json: unsupported type: chan int",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			response := tt.handler.admit(tt.review)
			if response.Allowed != tt.allowed {
				t.Fatalf("Allowed = %t, want %t", response.Allowed, tt.allowed)
			}
			if tt.message != "" {
				if response.Result == nil || response.Result.Message != tt.message {
					t.Fatalf("message = %#v, want %q", response.Result, tt.message)
				}
				return
			}
			if response.Result != nil {
				t.Fatalf("unexpected rejection message: %q", response.Result.Message)
			}
			if !tt.expectPatch {
				if response.Patch != nil || response.PatchType != nil {
					t.Fatalf("unexpected patch response: %#v", response)
				}
				return
			}
			if response.PatchType == nil || *response.PatchType != admissionv1.PatchTypeJSONPatch {
				t.Fatalf("PatchType = %#v, want JSONPatch", response.PatchType)
			}
			var patch []mutationAdapterTestPatch
			if err := json.Unmarshal(response.Patch, &patch); err != nil {
				t.Fatal(err)
			}
			if len(patch) != len(tt.patch) {
				t.Fatalf("patch = %#v, want %#v", patch, tt.patch)
			}
			for i := range patch {
				if patch[i].Op != tt.patch[i].Op || patch[i].Path != tt.patch[i].Path || patch[i].Value != tt.patch[i].Value {
					t.Fatalf("patch = %#v, want %#v", patch, tt.patch)
				}
			}
		})
	}
}

func TestMutationHandlerRejectsMissingRequest(t *testing.T) {
	handler := mutationHandler[mutationAdapterTestObject, mutationAdapterTestPatch]{}
	response := handler.admit(admissionv1.AdmissionReview{})

	if response.Allowed {
		t.Fatal("review without request was allowed")
	}
	if response.Result == nil || response.Result.Message != "admission review request is required" {
		t.Fatalf("message = %#v", response.Result)
	}
}

func TestMutationHandlerPassesOriginalObjectToRawMutation(t *testing.T) {
	raw := []byte(`{"value":"explicit"}`)
	handler := mutationHandler[mutationAdapterTestObject, mutationAdapterTestPatch]{
		newObject: func() *mutationAdapterTestObject { return &mutationAdapterTestObject{} },
		rawMutation: func(object *mutationAdapterTestObject, original []byte) ([]mutationAdapterTestPatch, error) {
			if object.Value != "explicit" || string(original) != string(raw) {
				t.Fatalf("object = %#v, original = %s", object, original)
			}
			return nil, nil
		},
	}

	response := handler.admit(admissionv1.AdmissionReview{Request: &admissionv1.AdmissionRequest{
		Object: runtime.RawExtension{Raw: raw},
	}})
	if !response.Allowed || response.Patch != nil {
		t.Fatalf("response = %#v", response)
	}
}

func TestServePreservesUIDAndMutationPatch(t *testing.T) {
	review := admissionv1.AdmissionReview{Request: &admissionv1.AdmissionRequest{
		UID:    types.UID("mutation-request-uid"),
		Object: runtime.RawExtension{Raw: []byte(`{"value":"default"}`)},
	}}
	body, err := json.Marshal(review)
	if err != nil {
		t.Fatal(err)
	}

	handler := mutationHandler[mutationAdapterTestObject, mutationAdapterTestPatch]{
		newObject: func() *mutationAdapterTestObject { return &mutationAdapterTestObject{} },
		mutation: func(*mutationAdapterTestObject) ([]mutationAdapterTestPatch, error) {
			return []mutationAdapterTestPatch{{Op: "add", Path: "/value", Value: "mutated"}}, nil
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	serve(recorder, request, handler.admit)

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
	if !responseReview.Response.Allowed || responseReview.Response.PatchType == nil || *responseReview.Response.PatchType != admissionv1.PatchTypeJSONPatch {
		t.Fatalf("response = %#v", responseReview.Response)
	}
}
