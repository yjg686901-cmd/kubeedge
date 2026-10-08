# PR: Remove standalone Admission and finish Controller Manager webhook unification

## Summary

This change removes the standalone Admission executable and deployment path
after the unified Controller Manager passed Week 5 cluster validation. It also
makes Helm the only owner of webhook registration resources and adds final
operations and contributor documentation.

## Changes

- Delete `cloud/cmd/admission` and its independent TLS server and options.
- Delete `build/admission`, the Admission image build/release entries, and its
  Deployment, ServiceAccount, ClusterRole, and ClusterRoleBinding.
- Remove process side ValidatingWebhookConfiguration and
  MutatingWebhookConfiguration registration.
- Remove legacy Admission values and templates from the CloudCore chart.
- Keep the compatible `kubeedge-admission-service` name, now selecting only
  Controller Manager replicas on port 9443.
- Remove the legacy Admission rollback path; rollback uses a prior unified Helm
  revision.
- Add architecture, installation, upgrade, rollback, troubleshooting, and
  contributor documentation.

## Migration

Before upgrading, create the webhook TLS Secret and set the Controller Manager
webhook values. Use `helm upgrade --atomic`. Confirm two ready replicas and two
Service endpoints. Roll back only to a previously validated unified revision.

## Verification

The reproducible commands and recorded outcomes are maintained in
`Week6-Final-Report.md`.
