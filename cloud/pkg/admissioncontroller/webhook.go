package admissioncontroller

import (
	"fmt"
	"net/http"

	restclient "k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/kubeedge/api/client/clientset/versioned"
)

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
