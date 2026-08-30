package admissioncontroller

import (
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"

	devicesv1beta1 "github.com/kubeedge/api/apis/devices/v1beta1"
)

func admitDevice(review admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	return validationHandler[devicesv1beta1.Device]{
		newObject: func() *devicesv1beta1.Device { return &devicesv1beta1.Device{} },
		validations: map[admissionv1.Operation]validationFunc[devicesv1beta1.Device]{
			admissionv1.Create:  func(device, _ *devicesv1beta1.Device) error { return validateDevice(device) },
			admissionv1.Update:  func(device, _ *devicesv1beta1.Device) error { return validateDevice(device) },
			admissionv1.Delete:  nil,
			admissionv1.Connect: nil,
		},
		unsupportedMessage: func(admissionv1.Operation) error {
			return fmt.Errorf("Unsupported webhook operation!")
		},
	}.admit(review)
}

func validateDevice(device *devicesv1beta1.Device) error {
	//device properties name must be unique.
	size := len(device.Spec.Properties)
	for i := range device.Spec.Properties {
		for j := i + 1; j < size; j++ {
			if device.Spec.Properties[i].Name == device.Spec.Properties[j].Name {
				return fmt.Errorf("property names must be unique.")
			}
		}
	}

	return nil
}

func serveDevice(w http.ResponseWriter, r *http.Request) {
	serve(w, r, admitDevice)
}
