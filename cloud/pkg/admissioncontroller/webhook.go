package admissioncontroller

import (
	"context"
	"fmt"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	restclient "k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	rulesv1 "github.com/kubeedge/api/apis/rules/v1"
	"github.com/kubeedge/api/client/clientset/versioned"
)

var controller = &AdmissionController{}

// AdmissionController contains the KubeEdge client required by handlers that
// validate cross-resource references. Webhook lifecycle and TLS are owned by
// controller-runtime.
type AdmissionController struct {
	CrdClient versioned.Interface
}

// RegisterWebhooks initializes dependencies and registers all KubeEdge
// admission handlers to the controller-runtime webhook server.
func RegisterWebhooks(server webhook.Server, restConfig *restclient.Config) error {
	crdClient, err := versioned.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create KubeEdge CRD client failed: %w", err)
	}

	controller.CrdClient = crdClient

	server.Register("/devices", http.HandlerFunc(serveDevice))
	server.Register("/devicemodels", http.HandlerFunc(serveDeviceModel))
	server.Register("/rules", http.HandlerFunc(serveRule))
	server.Register("/ruleendpoints", http.HandlerFunc(serveRuleEndpoint))
	server.Register("/nodeupgradejobs", http.HandlerFunc(serveNodeUpgradeJob))

	server.Register("/offlinemigration", http.HandlerFunc(serveOfflineMigration))
	server.Register("/mutating/nodeupgradejobs", http.HandlerFunc(serveMutatingNodeUpgradeJob))

	return nil
}

func (ac *AdmissionController) getRuleEndpoint(namespace, name string) (*rulesv1.RuleEndpoint, error) {
	return ac.CrdClient.RulesV1().RuleEndpoints(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

func (ac *AdmissionController) listRule(namespace string) ([]rulesv1.Rule, error) {
	rules, err := ac.CrdClient.RulesV1().Rules(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return rules.Items, nil
}

func (ac *AdmissionController) listRuleEndpoint(namespace string) ([]rulesv1.RuleEndpoint, error) {
	endpoints, err := ac.CrdClient.RulesV1().RuleEndpoints(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return endpoints.Items, nil
}
