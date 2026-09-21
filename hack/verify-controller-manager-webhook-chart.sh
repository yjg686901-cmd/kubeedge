#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
CHART="${REPO_ROOT}/manifests/charts/cloudcore"
WORK_DIR=$(mktemp -d)
trap 'rm -rf "${WORK_DIR}"' EXIT

helm lint "${CHART}"

helm template controller-test "${CHART}" \
  --namespace kubeedge \
  --set controllerManager.enable=true \
  >"${WORK_DIR}/controller.yaml"
grep -q 'path: /healthz' "${WORK_DIR}/controller.yaml"
grep -q 'path: /readyz' "${WORK_DIR}/controller.yaml"
if grep -q 'name: webhook$' "${WORK_DIR}/controller.yaml"; then
  echo 'disabled webhook unexpectedly rendered a webhook container port' >&2
  exit 1
fi

helm template webhook-test "${CHART}" \
  --namespace kubeedge \
  --set controllerManager.enable=true \
  --set controllerManager.webhook.enable=true \
  --set controllerManager.webhook.certsSecretName=kubeedge-webhook-certs \
  --set-string controllerManager.webhook.caBundle=Y2E= \
  >"${WORK_DIR}/controller-manager.yaml"

for path in /devices /devicemodels /rules /ruleendpoints /nodeupgradejobs /offlinemigration /mutating/nodeupgradejobs; do
  grep -q "path: ${path}$" "${WORK_DIR}/controller-manager.yaml"
done
grep -q 'targetPort: 9443' "${WORK_DIR}/controller-manager.yaml"
grep -q 'secretName: kubeedge-webhook-certs' "${WORK_DIR}/controller-manager.yaml"
grep -q 'path: /healthz' "${WORK_DIR}/controller-manager.yaml"
grep -q 'path: /readyz' "${WORK_DIR}/controller-manager.yaml"

helm template legacy-test "${CHART}" \
  --namespace kubeedge \
  --set admission.enable=true \
  >"${WORK_DIR}/legacy.yaml"
grep -q 'targetPort: 443' "${WORK_DIR}/legacy.yaml"
if grep -q 'kind: ValidatingWebhookConfiguration' "${WORK_DIR}/legacy.yaml"; then
  echo 'legacy backend unexpectedly rendered controller-manager webhook configurations' >&2
  exit 1
fi

assert_render_fails() {
  local expected=$1
  shift
  if helm template invalid-test "${CHART}" --namespace kubeedge "$@" >"${WORK_DIR}/invalid.out" 2>"${WORK_DIR}/invalid.err"; then
    echo 'invalid values unexpectedly rendered successfully' >&2
    exit 1
  fi
  grep -q "${expected}" "${WORK_DIR}/invalid.err"
}

assert_render_fails 'cannot both be true' \
  --set admission.enable=true \
  --set controllerManager.enable=true \
  --set controllerManager.webhook.enable=true
assert_render_fails 'controllerManager.enable must be true' \
  --set controllerManager.webhook.enable=true
assert_render_fails 'certsSecretName is required' \
  --set controllerManager.enable=true \
  --set controllerManager.webhook.enable=true
assert_render_fails 'caBundle is required' \
  --set controllerManager.enable=true \
  --set controllerManager.webhook.enable=true \
  --set controllerManager.webhook.certsSecretName=kubeedge-webhook-certs

echo 'controller-manager webhook chart verification passed'
