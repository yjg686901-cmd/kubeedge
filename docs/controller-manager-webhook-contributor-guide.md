# Controller Manager Webhook Contributor Guide

## Why the components were unified

The former Admission command duplicated process lifecycle, TLS serving,
clients, images, RBAC, and deployment resources. Controller Manager already
uses controller-runtime, which provides a managed webhook server with shared
health checks and shutdown handling. The unified design keeps the seven
existing paths and the compatible Service name while removing the second
process.

## Change checklist

1. Keep admission logic in `cloud/pkg/admissioncontroller` independent of HTTP
   server ownership.
2. Register paths through `RegisterWebhooks` on the Manager webhook server.
3. Define WebhookConfiguration rules in the CloudCore Helm chart.
4. Grant only the API permissions used by handlers and reconcilers.
5. Test valid, invalid, update, delete, mutation, idempotency, two replicas,
   leader switching, TLS failure, upgrade, and rollback.
6. Run the workflow in `.github/workflows/controller-manager-webhook.yml` and
   attach the run URL to the pull request.

## Compatibility rules

Preserve paths, operations, API groups and versions, failure policy, object
selectors, AdmissionReview `v1`, response UID, rejection meaning, JSON Patch
semantics, and the Service DNS name. Document any intentional behavior change
and add a migration test before changing one of these contracts.

## Review guide

Reviewers should reject a new standalone Admission command, runtime webhook
registration, a second TLS listener, broad RBAC, or a deployment path outside
the unified chart. Generated and vendor changes must match the pinned Go and
controller-runtime toolchain and pass the repository verification targets.
