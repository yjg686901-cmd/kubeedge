package admissioncontroller

import (
	"fmt"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"

	devicesv1beta1 "github.com/kubeedge/api/apis/devices/v1beta1"
)

func admitDeviceModel(review admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	return validationHandler[devicesv1beta1.DeviceModel]{
		newObject: func() *devicesv1beta1.DeviceModel { return &devicesv1beta1.DeviceModel{} },
		validations: map[admissionv1.Operation]validationFunc[devicesv1beta1.DeviceModel]{
			admissionv1.Create:  func(deviceModel, _ *devicesv1beta1.DeviceModel) error { return validateDeviceModel(deviceModel) },
			admissionv1.Update:  func(deviceModel, _ *devicesv1beta1.DeviceModel) error { return validateDeviceModel(deviceModel) },
			admissionv1.Delete:  nil,
			admissionv1.Connect: nil,
		},
		unsupportedMessage: func(admissionv1.Operation) error {
			return fmt.Errorf("Unsupported webhook operation!")
		},
	}.admit(review)
}

func validateDeviceModel(devicemodel *devicesv1beta1.DeviceModel) error {
	//device properties must be either Int or String while additional properties is not banned.
	propertyNameMap := make(map[string]bool)
	for _, property := range devicemodel.Spec.Properties {
		if _, ok := propertyNameMap[property.Name]; !ok {
			propertyNameMap[property.Name] = true
		} else {
			return fmt.Errorf("property names must be unique.")
		}
	}
	return nil
}

func serveDeviceModel(w http.ResponseWriter, r *http.Request) {
	serve(w, r, admitDeviceModel)
}
