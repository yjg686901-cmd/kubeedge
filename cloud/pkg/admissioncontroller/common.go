package admissioncontroller

import (
	"encoding/json"
	"io"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"

	devicesv1beta1 "github.com/kubeedge/api/apis/devices/v1beta1"
	"github.com/kubeedge/kubeedge/common/constants"
)

var admissionCodecs = func() serializer.CodecFactory {
	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(admissionv1.AddToScheme(scheme))
	utilruntime.Must(devicesv1beta1.AddDeviceCrds(scheme))
	return serializer.NewCodecFactory(scheme)
}()

// hookFunc is the type we use for all of our validators and mutators
type hookFunc func(admissionv1.AdmissionReview) *admissionv1.AdmissionResponse

func serve(w http.ResponseWriter, r *http.Request, hook hookFunc) {
	var body []byte
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, constants.MaxRespBodyLength)
		if data, err := io.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		klog.Errorf("contentType=%s, expect application/json", contentType)
		return
	}

	// The AdmissionReview that was sent to the webhook
	requestedAdmissionReview := admissionv1.AdmissionReview{}

	// The AdmissionReview that will be returned
	responseAdmissionReview := admissionv1.AdmissionReview{}
	responseAdmissionReview.SetGroupVersionKind(admissionv1.SchemeGroupVersion.WithKind("AdmissionReview"))

	deserializer := admissionCodecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(body, nil, &requestedAdmissionReview); err != nil {
		klog.Errorf("decode failed with error: %v", err)
		responseAdmissionReview.Response = toAdmissionResponse(err)
	} else {
		responseAdmissionReview.Response = hook(requestedAdmissionReview)
		// Return the same UID
		responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID
	}

	klog.V(4).Infof("sending response: %+v", responseAdmissionReview.Response)

	respBytes, err := json.Marshal(responseAdmissionReview)
	if err != nil {
		klog.Errorf("cannot marshal to a valid response %v", err)
		return
	}
	if _, err := w.Write(respBytes); err != nil {
		klog.Errorf("cannot write response %v", err)
		return
	}
}

// toAdmissionResponse is a helper function to create an AdmissionResponse
func toAdmissionResponse(err error) *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}
