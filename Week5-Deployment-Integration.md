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
- Kept Helm ownership of WebhookConfigurations during the legacy rollback
  overlay, so a later Helm rollback to the new backend can reuse them.

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
   If the legacy process created the three same-named WebhookConfigurations,
   audit their ownership first. Helm 3.17 can adopt unowned configurations on
   the initial dedicated-cluster upgrade with `--take-ownership`.
4. Wait for the Controller Manager Deployment readiness condition.
5. Run admission regression cases before removing the legacy image.

## Rollback procedure

Upgrade the release with `values-legacy-admission-rollback.yaml` while retaining
the CA bundle in the release values. Helm keeps ownership of the three
WebhookConfigurations and points the shared Service back to the legacy Admission
Deployment. Both backends use a Secret containing `tls.crt`, `tls.key`, and
`ca.crt`; the legacy process updates those configurations in place without
removing Helm metadata. A subsequent Helm rollback can restore the new backend.

## Test status (2026-09-22)

- Kubernetes 1.32 envtest: the complete Controller Manager suite passes (52/52).
  The direct TLS Webhook subset passes (8/8), and seven additional tests register
  WebhookConfigurations in envtest and trigger all seven routes through the real
  Kubernetes API Server. Context shutdown releases the webhook port. Envtest uses
  localhost URLs, so the cluster Service and DNS path still require E2E testing.
- Helm Deployment, Service, and transition configurations pass
  `hack/verify-controller-manager-webhook-chart.sh`.
- Linux amd64 and arm64 Controller Manager binaries compile locally.
- `.github/workflows/controller-manager-webhook.yml` gates Helm rendering,
  Vendor/generated files, govulncheck, and both architecture builds in CI.
- Local govulncheck execution completed with a failing result: 38 reachable
  vulnerabilities across four modules and the Go 1.23.12 standard library.
  The new CI gate will remain red until these dependency/toolchain findings are
  remediated or explicitly triaged.
- Vendor and code generation checks initially failed because their scripts used
  single-module vendoring against a Go workspace vendor tree. The scripts now
  use the workspace layout. `make verify-vendor` and `make verify-codegen` pass.
- `make verify-crds` passes after installing controller-gen v0.17.3.
- Unit tests and `go vet` pass. A Race Test found a concurrent stop/read of
  `TimeoutJob`; the job now uses an atomic stop state and a stop channel. The
  focused race suite passes after this fix.
- `hack/test-controller-manager-webhook-e2e.sh` exercises all seven routes
  through the Service, a restart, both replicas, wrong-CA rejection, Service
  endpoint loss and restoration, and a Helm transition to the legacy backend
  followed by rollback. It requires an already deployed release on a dedicated
  cluster and has not been run end to end here. It does not replace a Kubernetes
  API Server admission regression.
- A preview Controller Manager Deployment is now running on the existing
  `k8s-master` cluster with two Ready replicas and a dedicated preview Service.
  Seven direct TLS routes and a namespace-scoped real API Server Device webhook
  pass. A Leader Pod deletion transferred the Lease while the nonleader served
  10/10 Webhook requests. A wrong CABundle and a no-Endpoint outage produced
  the expected API Server failures, and recovery restored admission.
- Controller Manager now supports optional Leader Election, enabled by default
  in Helm for this component, with Lease RBAC. Both replicas serve Webhooks;
  only the Lease holder runs reconcilers.
- Preview uses a static binary mounted from the host into the existing CloudCore
  image. Final image-based Helm upgrade, rollback, uninstall/reinstall, and
  all seven business E2E cases remain unverified.
