# Controller Manager Webhook Operations

## Architecture

KubeEdge serves its five validating and two mutating admission paths from the
controller-runtime webhook server embedded in `controller-manager`.

```text
kube-apiserver
  -> kubeedge-admission-service:443
  -> controller-manager Pod:9443
  -> admissioncontroller handler
```

The Service name remains `kubeedge-admission-service` for API compatibility.
Helm owns the Service and both WebhookConfiguration objects. The process does
not create or update registration objects. Each ready replica serves TLS and
admission requests; leader election only controls reconcilers.

| Area | Source |
| --- | --- |
| Server startup | `cloud/cmd/controllermanager/app/controllermanager.go` |
| Handler registration | `cloud/pkg/admissioncontroller/webhook.go` |
| Deployment and probes | `manifests/charts/cloudcore/templates/deployment_controllermanager.yaml` |
| Service | `manifests/charts/cloudcore/templates/service_admission.yaml` |
| Registration | `manifests/charts/cloudcore/templates/*webhookconfiguration.yaml` |

## Install

Create a certificate whose SAN contains
`kubeedge-admission-service.kubeedge.svc`, then create the Secret:

```bash
hack/generate-controller-manager-webhook-certs.sh kubeedge /tmp/kubeedge-webhook-certs
kubectl -n kubeedge create secret tls kubeedge-webhook-certs \
  --cert=/tmp/kubeedge-webhook-certs/tls.crt \
  --key=/tmp/kubeedge-webhook-certs/tls.key

CA_BUNDLE=$(base64 -w0 /tmp/kubeedge-webhook-certs/ca.crt)
helm upgrade --install cloudcore manifests/charts/cloudcore \
  --namespace kubeedge --create-namespace \
  --set controllerManager.enable=true \
  --set controllerManager.webhook.enable=true \
  --set controllerManager.webhook.certsSecretName=kubeedge-webhook-certs \
  --set-string controllerManager.webhook.caBundle="$CA_BUNDLE"
```

Confirm both replicas and Service endpoints:

```bash
kubectl -n kubeedge rollout status deployment/kubeedge-controller-manager
kubectl -n kubeedge get deployment kubeedge-controller-manager
kubectl -n kubeedge get endpoints kubeedge-admission-service
kubectl get validatingwebhookconfiguration kubeedge-crds-validate-webhook-configuration
kubectl get mutatingwebhookconfiguration kubeedge-mutating-webhook mutate-offlinemigration
```

## Upgrade and rollback

Back up values and record the current revision before an upgrade:

```bash
helm -n kubeedge get values cloudcore -a > cloudcore-values-backup.yaml
helm -n kubeedge history cloudcore
helm upgrade cloudcore manifests/charts/cloudcore -n kubeedge --reuse-values --atomic --timeout 5m
```

Rollback targets a previous unified Controller Manager revision:

```bash
helm -n kubeedge rollback cloudcore REVISION --wait --timeout 5m
kubectl -n kubeedge rollout status deployment/kubeedge-controller-manager --timeout=5m
kubectl -n kubeedge get endpoints kubeedge-admission-service
```

The old standalone Admission binary and Deployment are no longer rollback
targets. Preserve at least one validated unified release revision before
removing older Helm history.

## Development and verification

Register a new handler in `RegisterWebhooks`, add its declarative rule to the
chart, and add unit, behavior comparison, Envtest, and E2E coverage. Run:

```bash
go test ./cloud/pkg/admissioncontroller/... ./cloud/pkg/controllermanager/...
go test -race ./cloud/pkg/admissioncontroller/... \
  ./cloud/pkg/controllermanager/nodegroup/... \
  ./cloud/pkg/controllermanager/nodetask/...
go vet ./cloud/pkg/admissioncontroller/... ./cloud/pkg/controllermanager/...
hack/verify-controller-manager-webhook-chart.sh
make verify-vendor verify-codegen verify-crds
```

Use `admission-baseline/cases.yaml` and its fixtures when checking compatibility.
Do not add process side registration or a second TLS server.

## Troubleshooting

| Symptom | Check | Resolution |
| --- | --- | --- |
| `no endpoints available` | Pod readiness, Service selector, endpoint list | Fix readiness or selector; wait for ready replicas |
| `x509` or handshake error | Secret certificate SAN and Webhook `caBundle` | Regenerate the certificate and upgrade the release with the matching CA |
| connection refused | Container port 9443, Service `targetPort`, logs | Restore port mapping and certificate mount |
| denied request | Controller Manager logs and object validation error | Correct the object; keep `failurePolicy: Fail` |
| only one endpoint | replica readiness and scheduling events | Restore the second replica before disruptive work |
| Helm upgrade fails | `helm history`, rendered manifest, events | Fix values or chart, then retry with `--atomic`; rollback to the last unified revision |

Collect evidence with:

```bash
helm -n kubeedge history cloudcore
kubectl -n kubeedge get deploy,svc,endpoints,pods -o wide
kubectl -n kubeedge logs deployment/kubeedge-controller-manager --all-containers --tail=200
kubectl get validatingwebhookconfiguration,mutatingwebhookconfiguration -o yaml
```
