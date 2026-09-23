package controllermanager

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("Kubernetes API Server admission", func() {
	var caBundle []byte
	var failurePolicy = admissionregistrationv1.Fail
	var sideEffects = admissionregistrationv1.SideEffectClassNone
	var timeoutSeconds int32 = 5

	BeforeEach(func() {
		var err error
		caBundle, err = os.ReadFile(filepath.Join(webhookCertDir, "tls.crt"))
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() bool {
			resp, err := httpClient.Get(fmt.Sprintf("http://127.0.0.1:%d/readyz", healthProbePort))
			if err != nil {
				return false
			}
			defer resp.Body.Close()
			return resp.StatusCode == 200
		}, 20*time.Second, 200*time.Millisecond).Should(BeTrue())
	})

	It("applies the offline migration patch to a Pod created through the API Server", func() {
		path := "/offlinemigration"
		url := fmt.Sprintf("https://localhost:%d%s", webhookPort, path)
		config := &admissionregistrationv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "week5-envtest-offline-migration"},
			Webhooks: []admissionregistrationv1.MutatingWebhook{{
				Name:                    "offlinemigration.week5.kubeedge.io",
				AdmissionReviewVersions: []string{"v1"},
				SideEffects:             &sideEffects,
				FailurePolicy:           &failurePolicy,
				TimeoutSeconds:          &timeoutSeconds,
				ClientConfig:            admissionregistrationv1.WebhookClientConfig{URL: &url, CABundle: caBundle},
				ObjectSelector:          &metav1.LabelSelector{MatchLabels: map[string]string{"app-offline.kubeedge.io": "autonomy"}},
				Rules: []admissionregistrationv1.RuleWithOperations{{
					Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
					Rule:       admissionregistrationv1.Rule{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
				}},
			}},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, config)).To(Succeed()) })

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "week5-api-server-mutation", Namespace: "default", Labels: map[string]string{"app-offline.kubeedge.io": "autonomy"}},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "test", Image: "busybox"}}},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, pod)).To(Succeed()) })
		Expect(pod.Spec.Tolerations).To(ContainElement(SatisfyAll(
			HaveField("Key", corev1.TaintNodeUnreachable),
			HaveField("Operator", corev1.TolerationOpExists),
		)))
	})

	It("rejects an invalid Device created through the API Server", func() {
		path := "/devices"
		url := fmt.Sprintf("https://localhost:%d%s", webhookPort, path)
		config := &admissionregistrationv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "week5-envtest-device-validation"},
			Webhooks: []admissionregistrationv1.ValidatingWebhook{{
				Name:                    "device.week5.kubeedge.io",
				AdmissionReviewVersions: []string{"v1"},
				SideEffects:             &sideEffects,
				FailurePolicy:           &failurePolicy,
				TimeoutSeconds:          &timeoutSeconds,
				ClientConfig:            admissionregistrationv1.WebhookClientConfig{URL: &url, CABundle: caBundle},
				Rules: []admissionregistrationv1.RuleWithOperations{{
					Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
					Rule:       admissionregistrationv1.Rule{APIGroups: []string{"devices.kubeedge.io"}, APIVersions: []string{"v1beta1"}, Resources: []string{"devices"}},
				}},
			}},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, config)).To(Succeed()) })

		device := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "devices.kubeedge.io/v1beta1", "kind": "Device",
			"metadata": map[string]interface{}{"name": "week5-invalid-device", "namespace": "default"},
			"spec": map[string]interface{}{"properties": []interface{}{
				map[string]interface{}{"name": "duplicate"}, map[string]interface{}{"name": "duplicate"},
			}},
		}}
		err := k8sClient.Create(ctx, device)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("admission webhook \"device.week5.kubeedge.io\" denied the request"))
		Expect(err.Error()).To(ContainSubstring("property names must be unique"))
	})

	It("rejects duplicate DeviceModel properties through the API Server", func() {
		registerValidatingRoute(caBundle, "devicemodel", "/devicemodels", "devices.kubeedge.io", "v1beta1", "devicemodels")
		model := admissionResource("devices.kubeedge.io/v1beta1", "DeviceModel", "week5-invalid-model", map[string]interface{}{
			"properties": []interface{}{map[string]interface{}{"name": "duplicate"}, map[string]interface{}{"name": "duplicate"}},
		})
		err := k8sClient.Create(ctx, model)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("admission webhook \"devicemodel.week5.kubeedge.io\" denied the request"))
		Expect(err.Error()).To(ContainSubstring("property names must be unique"))
	})

	It("rejects a Rule without source RuleEndpoint through the API Server", func() {
		registerValidatingRoute(caBundle, "rule", "/rules", "rules.kubeedge.io", "v1", "rules")
		rule := admissionResource("rules.kubeedge.io/v1", "Rule", "week5-invalid-rule", map[string]interface{}{
			"source": "missing-source", "sourceResource": map[string]interface{}{},
			"target": "missing-target", "targetResource": map[string]interface{}{},
		})
		err := k8sClient.Create(ctx, rule)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("admission webhook \"rule.week5.kubeedge.io\" denied the request"))
		Expect(err.Error()).To(ContainSubstring("source ruleEndpoint"))
	})

	It("rejects an invalid servicebus RuleEndpoint through the API Server", func() {
		registerValidatingRoute(caBundle, "ruleendpoint", "/ruleendpoints", "rules.kubeedge.io", "v1", "ruleendpoints")
		endpoint := admissionResource("rules.kubeedge.io/v1", "RuleEndpoint", "week5-invalid-endpoint", map[string]interface{}{
			"ruleEndpointType": "servicebus", "properties": map[string]interface{}{},
		})
		err := k8sClient.Create(ctx, endpoint)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("admission webhook \"ruleendpoint.week5.kubeedge.io\" denied the request"))
		Expect(err.Error()).To(ContainSubstring("service_port"))
	})

	It("rejects an invalid NodeUpgradeJob through the API Server", func() {
		registerValidatingRoute(caBundle, "nodeupgradejob", "/nodeupgradejobs", "operations.kubeedge.io", "v1alpha1", "nodeupgradejobs")
		job := admissionResource("operations.kubeedge.io/v1alpha1", "NodeUpgradeJob", "week5-invalid-upgrade", map[string]interface{}{
			"version": "1.0.0", "nodeNames": []interface{}{"node1"},
		})
		job.SetNamespace("")
		err := k8sClient.Create(ctx, job)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("admission webhook \"nodeupgradejob.week5.kubeedge.io\" denied the request"))
		Expect(err.Error()).To(ContainSubstring("invalid version"))
	})

	It("applies NodeUpgradeJob defaults through the API Server", func() {
		url := fmt.Sprintf("https://localhost:%d/mutating/nodeupgradejobs", webhookPort)
		config := &admissionregistrationv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "week5-envtest-upgrade-mutation"},
			Webhooks: []admissionregistrationv1.MutatingWebhook{{
				Name: "upgrade-mutation.week5.kubeedge.io", AdmissionReviewVersions: []string{"v1"},
				SideEffects: &sideEffects, FailurePolicy: &failurePolicy, TimeoutSeconds: &timeoutSeconds,
				ClientConfig: admissionregistrationv1.WebhookClientConfig{URL: &url, CABundle: caBundle},
				Rules: []admissionregistrationv1.RuleWithOperations{{
					Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
					Rule:       admissionregistrationv1.Rule{APIGroups: []string{"operations.kubeedge.io"}, APIVersions: []string{"v1alpha1"}, Resources: []string{"nodeupgradejobs"}},
				}},
			}},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, config)).To(Succeed()) })
		job := admissionResource("operations.kubeedge.io/v1alpha1", "NodeUpgradeJob", "week5-default-upgrade", map[string]interface{}{
			"version": "v1.0.0", "nodeNames": []interface{}{"node1"},
		})
		job.SetNamespace("")
		Expect(k8sClient.Create(ctx, job)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, job)).To(Succeed()) })
		concurrency, found, err := unstructured.NestedInt64(job.Object, "spec", "concurrency")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(concurrency).To(Equal(int64(1)))
		timeout, found, err := unstructured.NestedInt64(job.Object, "spec", "timeoutSeconds")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(timeout).To(Equal(int64(300)))
	})
})

func admissionResource(apiVersion, kind, name string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion, "kind": kind,
		"metadata": map[string]interface{}{"name": name, "namespace": "default"},
		"spec":     spec,
	}}
}

func registerValidatingRoute(caBundle []byte, name, path, group, version, resource string) {
	failurePolicy := admissionregistrationv1.Fail
	sideEffects := admissionregistrationv1.SideEffectClassNone
	timeoutSeconds := int32(5)
	url := fmt.Sprintf("https://localhost:%d%s", webhookPort, path)
	config := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "week5-envtest-" + name},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{{
			Name: name + ".week5.kubeedge.io", AdmissionReviewVersions: []string{"v1"},
			SideEffects: &sideEffects, FailurePolicy: &failurePolicy, TimeoutSeconds: &timeoutSeconds,
			ClientConfig: admissionregistrationv1.WebhookClientConfig{URL: &url, CABundle: caBundle},
			Rules: []admissionregistrationv1.RuleWithOperations{{
				Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
				Rule:       admissionregistrationv1.Rule{APIGroups: []string{group}, APIVersions: []string{version}, Resources: []string{resource}},
			}},
		}},
	}
	Expect(k8sClient.Create(ctx, config)).To(Succeed())
	DeferCleanup(func() { Expect(k8sClient.Delete(ctx, config)).To(Succeed()) })
}

var httpClient = http.Client{Timeout: 2 * time.Second}
