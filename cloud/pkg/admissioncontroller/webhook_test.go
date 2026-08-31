package admissioncontroller

import (
	"net/http"
	"testing"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func TestRegisterWebhooks(t *testing.T) {
	server := webhook.NewServer(webhook.Options{})
	kubeCfg := &rest.Config{Host: "https://127.0.0.1"}

	if err := RegisterWebhooks(server, kubeCfg); err != nil {
		t.Fatalf("failed to register webhooks: %v", err)
	}

	paths := []string{
		"/devices",
		"/devicemodels",
		"/rules",
		"/ruleendpoints",
		"/nodeupgradejobs",
		"/offlinemigration",
		"/mutating/nodeupgradejobs",
	}

	mux := server.WebhookMux()

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, path, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			handler, pattern := mux.Handler(req)

			if handler == nil {
				t.Fatalf("no handler registered for path %s", path)
			}

			if pattern != path {
				t.Fatalf(
					"unexpected registered pattern for %s: got %q",
					path,
					pattern,
				)
			}
		})
	}
}

func TestRegisterWebhooksDuplicateRegistrationPanics(t *testing.T) {
	server := webhook.NewServer(webhook.Options{})
	kubeCfg := &rest.Config{Host: "https://127.0.0.1"}

	if err := RegisterWebhooks(server, kubeCfg); err != nil {
		t.Fatalf("failed to register webhooks: %v", err)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic when registering duplicate webhook paths")
		}
	}()

	_ = RegisterWebhooks(server, kubeCfg)
}
