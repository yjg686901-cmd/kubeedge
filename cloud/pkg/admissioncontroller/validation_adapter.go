package admissioncontroller

import (
	"encoding/json"
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// validationFunc contains only resource validation. AdmissionReview decoding and
// AdmissionResponse construction are handled by validationHandler.
type validationFunc[T any] func(newObject, oldObject *T) error

type validationHandler[T any] struct {
	newObject          func() *T
	validations        map[admissionv1.Operation]validationFunc[T]
	requiresOldObject  map[admissionv1.Operation]bool
	decodeError        func(error) error
	unsupportedMessage func(admissionv1.Operation) error
}

func (h validationHandler[T]) admit(review admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	if review.Request == nil {
		return admissionResponse(fmt.Errorf("admission review request is required"))
	}

	validate, supported := h.validations[review.Request.Operation]
	if !supported {
		return admissionResponse(h.unsupported(review.Request.Operation))
	}
	if validate == nil {
		return admissionResponse(nil)
	}

	newObject := h.newObject()
	if err := json.Unmarshal(review.Request.Object.Raw, newObject); err != nil {
		return admissionResponse(h.decodeFailure(err))
	}

	var oldObject *T
	if h.requiresOldObject[review.Request.Operation] {
		oldObject = h.newObject()
		if err := json.Unmarshal(review.Request.OldObject.Raw, oldObject); err != nil {
			return admissionResponse(h.decodeFailure(err))
		}
	}

	return admissionResponse(validate(newObject, oldObject))
}

func (h validationHandler[T]) decodeFailure(err error) error {
	if h.decodeError != nil {
		return h.decodeError(err)
	}
	return err
}

func (h validationHandler[T]) unsupported(operation admissionv1.Operation) error {
	if h.unsupportedMessage != nil {
		return h.unsupportedMessage(operation)
	}
	return fmt.Errorf("unsupported webhook operation %v", operation)
}

func admissionResponse(err error) *admissionv1.AdmissionResponse {
	if err != nil {
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result:  &metav1.Status{Message: err.Error()},
		}
	}
	return &admissionv1.AdmissionResponse{Allowed: true}
}
