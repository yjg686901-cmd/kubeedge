#!/usr/bin/env bash
# Run only against a dedicated cluster with a deployed Controller Manager webhook.
set -euo pipefail

: "${WEEK5_E2E_CONTEXT:?Set the dedicated kubeconfig context}"
: "${WEEK5_E2E_NAMESPACE:?Set the release namespace}"
: "${WEEK5_E2E_RELEASE:?Set the Helm release name}"
: "${WEEK5_E2E_CA_FILE:?Set the serving certificate CA file}"
[[ "${WEEK5_E2E_DEDICATED:-}" == 1 ]] || { echo 'Set WEEK5_E2E_DEDICATED=1 after checking the target cluster' >&2; exit 2; }

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d)
service=kubeedge-admission-service
deployment=kubeedge-controller-manager
host="${service}.${WEEK5_E2E_NAMESPACE}.svc"
port_forward_pid=''
selector=''
replicas=''
revision=''
reinstall_needed=0

k() { kubectl --context "${WEEK5_E2E_CONTEXT}" -n "${WEEK5_E2E_NAMESPACE}" "$@"; }
h() { helm --kube-context "${WEEK5_E2E_CONTEXT}" -n "${WEEK5_E2E_NAMESPACE}" "$@"; }
stop_forward() {
  if [[ -n "${port_forward_pid}" ]]; then
    kill "${port_forward_pid}" 2>/dev/null || true
    wait "${port_forward_pid}" 2>/dev/null || true
    port_forward_pid=''
  fi
}
cleanup() {
  local keep_work=0
  stop_forward
  if [[ -n "${selector}" ]]; then
    k patch service "${service}" --type json -p "$(jq -nc --argjson value "${selector}" '[{op:"replace",path:"/spec/selector",value:$value}]')" >/dev/null || true
  fi
  if [[ -n "${replicas}" ]]; then
    k scale deployment "${deployment}" --replicas="${replicas}" >/dev/null || true
  fi
  if [[ -n "${revision}" ]]; then
    h rollback "${WEEK5_E2E_RELEASE}" "${revision}" --wait --timeout=3m >/dev/null || true
  fi
  if [[ "${reinstall_needed}" == 1 ]]; then
    if ! h install "${WEEK5_E2E_RELEASE}" "${root}/manifests/charts/cloudcore" \
      -f "${work}/release-values.yaml" --wait --timeout=3m >/dev/null; then
      keep_work=1
      echo "Automatic reinstall failed; restore the dedicated release using ${work}/release-values.yaml" >&2
    fi
  fi
  if [[ "${keep_work}" == 0 ]]; then
    rm -rf "${work}"
  fi
}
trap cleanup EXIT

k get deployment "${deployment}" >/dev/null
k get service "${service}" >/dev/null
h status "${WEEK5_E2E_RELEASE}" >/dev/null
release_status=$(h status "${WEEK5_E2E_RELEASE}" -o json | jq -r '.info.status')
[[ "${release_status}" == deployed ]] || {
  echo "Helm release ${WEEK5_E2E_RELEASE} is ${release_status}; establish a deployed baseline before E2E" >&2
  exit 2
}
replicas=$(k get deployment "${deployment}" -o jsonpath='{.spec.replicas}')
selector=$(k get service "${service}" -o json | jq -c '.spec.selector')
revision=$(h history "${WEEK5_E2E_RELEASE}" -o json | jq -r '.[-1].revision')
h get values "${WEEK5_E2E_RELEASE}" --all -o yaml >"${work}/release-values.yaml"
k rollout status deployment "${deployment}" --timeout=3m

start_forward() {
  local resource=${1:-service/${service}} remote_port=${2:-443}
  stop_forward
  k port-forward "${resource}" "18443:${remote_port}" >"${work}/port-forward.log" 2>&1 &
  port_forward_pid=$!
  for _ in $(seq 1 30); do
    if curl -sS --noproxy '*' --cacert "${WEEK5_E2E_CA_FILE}" --connect-to "${host}:443:127.0.0.1:18443" "https://${host}/devices" >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done
  cat "${work}/port-forward.log" >&2
  echo 'Webhook Service did not become reachable' >&2
  exit 1
}

request() {
  local path=$1 operation=$2 object=$3 want_patch=$4
  jq -nc --arg op "${operation}" --argjson obj "${object}" \
    '{apiVersion:"admission.k8s.io/v1",kind:"AdmissionReview",request:{uid:"week5-e2e",operation:$op,object:{raw:$obj}}}' \
    >"${work}/review.json"
  # AdmissionReview.RawExtension is encoded as the object itself on the wire.
  jq '.request.object = .request.object.raw' "${work}/review.json" >"${work}/wire.json"
  curl -fsS --noproxy '*' --cacert "${WEEK5_E2E_CA_FILE}" \
    --connect-to "${host}:443:127.0.0.1:18443" \
    -H 'Content-Type: application/json' --data-binary @"${work}/wire.json" \
    "https://${host}${path}" | jq -e --argjson patch "${want_patch}" \
    '.response.uid == "week5-e2e" and .response.allowed == true and (if $patch then .response.patchType == "JSONPatch" else true end)' >/dev/null
}

# This checks TLS reachability and handler registration. E01-E07 additionally
# require Kubernetes API Server business requests, which are separate tests.
route_smoke() {
  request /devices DELETE '{}' false
  request /devicemodels DELETE '{}' false
  request /rules DELETE '{}' false
  request /ruleendpoints DELETE '{}' false
  request /nodeupgradejobs DELETE '{}' false
  request /offlinemigration CREATE '{"spec":{}}' true
  request /mutating/nodeupgradejobs CREATE '{"spec":{}}' true
}

invalid_device_rejected() {
  local output
  if output=$(k create --dry-run=server -f - 2>&1 <<YAML
apiVersion: devices.kubeedge.io/v1beta1
kind: Device
metadata:
  name: week5-e2e-invalid-device
  namespace: ${WEEK5_E2E_NAMESPACE}
spec:
  properties:
    - name: duplicate
    - name: duplicate
YAML
); then
    echo 'Invalid Device was admitted by the API Server' >&2
    return 1
  fi
  printf '%s\n' "${output}" | grep -q 'property names must be unique' || {
    printf '%s\n' "${output}" >&2
    echo 'Invalid Device failed for a reason other than the expected webhook validation' >&2
    return 1
  }
}

start_forward
route_smoke
invalid_device_rejected

# A client with the wrong CA must reject the service certificate.
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=wrong-ca' \
  -keyout "${work}/wrong.key" -out "${work}/wrong.crt" >/dev/null 2>&1
if curl -sS --noproxy '*' --cacert "${work}/wrong.crt" \
  --connect-to "${host}:443:127.0.0.1:18443" "https://${host}/devices" >/dev/null 2>&1; then
  echo 'Webhook accepted an unrelated CA' >&2
  exit 1
fi

k rollout restart deployment "${deployment}"
k rollout status deployment "${deployment}" --timeout=3m
start_forward
route_smoke

k scale deployment "${deployment}" --replicas=2
k rollout status deployment "${deployment}" --timeout=3m
[[ "$(k get deployment "${deployment}" -o jsonpath='{.status.readyReplicas}')" == 2 ]]
pod_selector=$(k get deployment "${deployment}" -o json | jq -r '.spec.selector.matchLabels | to_entries | map(.key + "=" + .value) | join(",")')
for pod in $(k get pods -l "${pod_selector}" -o jsonpath='{.items[*].metadata.name}'); do
  start_forward "pod/${pod}" 9443
  route_smoke
done

stop_forward
k patch service "${service}" --type json -p '[{"op":"replace","path":"/spec/selector","value":{"week5-e2e-unavailable":"true"}}]' >/dev/null
for _ in $(seq 1 30); do
  [[ "$(k get endpoints "${service}" -o json | jq '[.subsets[]?.addresses[]?] | length')" == 0 ]] && break
  sleep 1
done
[[ "$(k get endpoints "${service}" -o json | jq '[.subsets[]?.addresses[]?] | length')" == 0 ]]
k patch service "${service}" --type json -p "$(jq -nc --argjson value "${selector}" '[{op:"replace",path:"/spec/selector",value:$value}]')" >/dev/null
selector=''
start_forward
route_smoke

# Roll back and restore the unified Controller Manager release.
stop_forward
h rollback "${WEEK5_E2E_RELEASE}" "${revision}" --wait --timeout=3m
revision=''
k rollout status deployment "${deployment}" --timeout=3m
start_forward
route_smoke
invalid_device_rejected

# E14 is destructive and must be requested separately on the dedicated test
# release. Values are saved before uninstall so the cleanup trap can recover.
if [[ "${WEEK5_E2E_ALLOW_UNINSTALL:-}" == 1 ]]; then
  stop_forward
  h uninstall "${WEEK5_E2E_RELEASE}" --wait --timeout=3m
  reinstall_needed=1
  for config in kubeedge-crds-validate-webhook-configuration mutate-offlinemigration kubeedge-mutating-webhook; do
    if kubectl --context "${WEEK5_E2E_CONTEXT}" get validatingwebhookconfiguration "${config}" >/dev/null 2>&1 ||
       kubectl --context "${WEEK5_E2E_CONTEXT}" get mutatingwebhookconfiguration "${config}" >/dev/null 2>&1; then
      echo "WebhookConfiguration ${config} remained after Helm uninstall" >&2
      exit 1
    fi
  done
  k create --dry-run=server -f - >/dev/null <<YAML
apiVersion: devices.kubeedge.io/v1beta1
kind: Device
metadata:
  name: week5-e14-after-uninstall
  namespace: ${WEEK5_E2E_NAMESPACE}
spec:
  properties: []
YAML
  h install "${WEEK5_E2E_RELEASE}" "${root}/manifests/charts/cloudcore" \
    -f "${work}/release-values.yaml" --wait --timeout=3m
  reinstall_needed=0
  k rollout status deployment "${deployment}" --timeout=3m
  start_forward
  route_smoke
  invalid_device_rejected
fi
echo 'Controller Manager-only webhook route, rollback, and reinstall smoke passed'
