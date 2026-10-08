#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
CHART="${ROOT}/manifests/charts/cloudcore"
WORK=$(mktemp -d)
trap 'rm -rf "${WORK}"' EXIT

helm lint "${CHART}"
helm template disabled "${CHART}" --namespace kubeedge >"${WORK}/disabled.yaml"
! grep -q 'name: kubeedge-admission-service$' "${WORK}/disabled.yaml"
! grep -q 'kind: ValidatingWebhookConfiguration' "${WORK}/disabled.yaml"
! grep -q 'name: kubeedge-admission$' "${WORK}/disabled.yaml"

helm template enabled "${CHART}" --namespace kubeedge \
  --set controllerManager.enable=true \
  --set controllerManager.webhook.enable=true \
  --set controllerManager.webhook.certsSecretName=kubeedge-webhook-certs \
  --set-string controllerManager.webhook.caBundle=Y2E= >"${WORK}/enabled.yaml"

for path in /devices /devicemodels /rules /ruleendpoints /nodeupgradejobs /offlinemigration /mutating/nodeupgradejobs; do
  grep -q "path: ${path}$" "${WORK}/enabled.yaml"
done
grep -q 'replicas: 2' "${WORK}/enabled.yaml"
grep -q 'targetPort: 9443' "${WORK}/enabled.yaml"
grep -q 'secretName: kubeedge-webhook-certs' "${WORK}/enabled.yaml"
grep -q 'path: /healthz' "${WORK}/enabled.yaml"
grep -q 'path: /readyz' "${WORK}/enabled.yaml"
grep -q -- '--leader-elect=true' "${WORK}/enabled.yaml"
grep -q 'coordination.k8s.io' "${WORK}/enabled.yaml"
! grep -q 'name: kubeedge-admission$' "${WORK}/enabled.yaml"

assert_fails() {
  local expected=$1; shift
  if helm template invalid "${CHART}" --namespace kubeedge "$@" >"${WORK}/out" 2>"${WORK}/err"; then
    echo 'invalid values unexpectedly rendered' >&2; exit 1
  fi
  grep -q "${expected}" "${WORK}/err"
}
assert_fails 'controllerManager.enable must be true' --set controllerManager.webhook.enable=true
assert_fails 'certsSecretName is required' --set controllerManager.enable=true --set controllerManager.webhook.enable=true
assert_fails 'caBundle is required' --set controllerManager.enable=true --set controllerManager.webhook.enable=true --set controllerManager.webhook.certsSecretName=kubeedge-webhook-certs

echo 'controller-manager-only webhook chart verification passed'
