# Week 5 Deployment Integration

## Completed implementation

- Added an explicit switch for serving admission webhooks from Controller Manager.
- Mounted an externally managed TLS Secret and exposed the webhook on port 9443.
- Routed `kubeedge-admission-service:443` to the selected admission backend.
- Added liveness and readiness probes; readiness includes webhook server startup.
- Rejected invalid Helm combinations, including simultaneous legacy and new backends.
- Rendered five validating and two mutating webhook routes with AdmissionReview v1.
- Removed the development-only `/test-admission` endpoint.
- Added certificate generation, cutover, and rollback artifacts.

## Cutover artifacts

- `hack/generate-controller-manager-webhook-certs.sh` generates a CA and serving
  certificate with all required Service DNS names.
- `manifests/charts/cloudcore/values-controller-manager-webhook.yaml` enables the
  Controller Manager backend.
- `manifests/charts/cloudcore/values-legacy-admission-rollback.yaml` restores the
  legacy Admission backend.
- `hack/verify-controller-manager-webhook-chart.sh` checks Helm rendering and
  invalid configuration handling.

## Cutover procedure

1. Generate or obtain a serving certificate for
   `kubeedge-admission-service.<namespace>.svc`.
2. Create the `kubeedge-webhook-certs` TLS Secret in the release namespace.
3. Upgrade with `values-controller-manager-webhook.yaml` and provide the CA bundle.
4. Wait for the Controller Manager Deployment readiness condition.
5. Run admission regression cases before removing the legacy image.

## Rollback procedure

Upgrade the release with `values-legacy-admission-rollback.yaml`. Helm removes the
Controller Manager webhook configurations and points the shared Service back to
the legacy Admission Deployment.

## Deferred verification

The Kubernetes 1.32 envtest suite, seven-route API Server regression, image build,
multi-architecture verification, vulnerability scan, and live cutover exercise
are deferred to the testing and environment execution stage.
