package admissioncontroller

import (
	"encoding/json"
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
)

// mutationFunc contains only resource mutation. AdmissionReview decoding and
// AdmissionResponse construction are handled by mutationHandler.
type mutationFunc[T any, P any] func(object *T) ([]P, error)

// rawMutationFunc additionally receives the original object JSON when a mutation
// needs to distinguish an omitted field from its Go zero value.
type rawMutationFunc[T any, P any] func(object *T, rawObject []byte) ([]P, error)

type mutationHandler[T any, P any] struct {
	newObject     func() *T
	mutation      mutationFunc[T, P]
	rawMutation   rawMutationFunc[T, P]
	decodeError   func(error) error
	mutationError func(error) error
}

func (h mutationHandler[T, P]) admit(review admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	if review.Request == nil {
		return admissionResponse(fmt.Errorf("admission review request is required"))
	}

	object := h.newObject()
	if err := json.Unmarshal(review.Request.Object.Raw, object); err != nil {
		return admissionResponse(h.decodeFailure(err))
	}

	var payload []P
	var err error
	if h.rawMutation != nil {
		payload, err = h.rawMutation(object, review.Request.Object.Raw)
	} else {
		payload, err = h.mutation(object)
	}
	if err != nil {
		return admissionResponse(h.mutationFailure(err))
	}
	if len(payload) == 0 {
		return admissionResponse(nil)
	}

	patch, err := json.Marshal(payload)
	if err != nil {
		return admissionResponse(h.mutationFailure(err))
	}

	patchType := admissionv1.PatchTypeJSONPatch
	return &admissionv1.AdmissionResponse{
		Allowed:   true,
		Patch:     patch,
		PatchType: &patchType,
	}
}

func (h mutationHandler[T, P]) decodeFailure(err error) error {
	if h.decodeError != nil {
		return h.decodeError(err)
	}
	return err
}

func (h mutationHandler[T, P]) mutationFailure(err error) error {
	if h.mutationError != nil {
		return h.mutationError(err)
	}
	return err
}
