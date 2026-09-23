#!/usr/bin/env bash

# Register preview webhooks only for resources bearing the Week 5 test label.
set -euo pipefail

context=${WEEK5_BUSINESS_CONTEXT:-kubernetes-admin@kubernetes}
ca_file=${WEEK5_BUSINESS_CA_FILE:-/home/ubuntu/week5-transfer/preview-certs/ca.crt}
cert_file=${WEEK5_BUSINESS_CERT_FILE:-/home/ubuntu/week5-transfer/preview-certs/tls.crt}
test_namespace=week5-business-test
service=kubeedge-controller-manager-webhook-preview

for binary in kubectl jq openssl base64; do
  command -v "$binary" >/dev/null || { echo "Missing command: $binary" >&2; exit 2; }
done

current_context=$(kubectl config current-context)
[[ "$current_context" == "$context" ]] || {
  echo "Current context is $current_context; expected $context" >&2
  exit 2
}

k() { kubectl --context "$context" "$@"; }

[[ -s "$ca_file" && -s "$cert_file" ]] || {
  echo "Missing CA or serving certificate: $ca_file / $cert_file" >&2
  exit 2
}
openssl x509 -in "$cert_file" -checkend 3600 -noout >/dev/null || {
  echo "Serving certificate expires within one hour: $cert_file" >&2
  exit 2
}
openssl verify -CAfile "$ca_file" "$cert_file" >/dev/null || {
  echo "Serving certificate is not signed by the supplied CA" >&2
  exit 2
}

ready=$(k -n kubeedge get deployment kubeedge-controller-manager -o jsonpath='{.status.readyReplicas}')
endpoints=$(k -n kubeedge get endpoints "$service" -o json | jq '[.subsets[]?.addresses[]?] | length')
[[ "$ready" == 2 && "$endpoints" -ge 2 ]] || {
  echo "Preview is not ready: readyReplicas=$ready endpoints=$endpoints" >&2
  exit 2
}

k get crd \
  devices.devices.kubeedge.io \
  devicemodels.devices.kubeedge.io \
  rules.rules.kubeedge.io \
  ruleendpoints.rules.kubeedge.io \
  nodeupgradejobs.operations.kubeedge.io \
  imageprepulljobs.operations.kubeedge.io \
  configupdatejobs.operations.kubeedge.io >/dev/null

for crd in nodeupgradejobs.operations.kubeedge.io imageprepulljobs.operations.kubeedge.io configupdatejobs.operations.kubeedge.io; do
  k get crd "${crd}" -o json | jq -e \
    '.spec.versions[] | select(.name == "v1alpha2" and .served == true)' >/dev/null || {
      echo "Required served version v1alpha2 is missing from CRD ${crd}" >&2
      exit 1
    }
done

ca_b64=$(base64 -w0 "$ca_file")
k create namespace "$test_namespace" --dry-run=client -o yaml | k apply -f -

jq -n --arg ca "$ca_b64" '
  def hook($name; $path; $group; $version; $resource; $ops):
    {name:$name,admissionReviewVersions:["v1"],sideEffects:"None",failurePolicy:"Fail",timeoutSeconds:5,
     objectSelector:{matchLabels:{"week5.kubeedge.io/business-test":"true"}},
     clientConfig:{service:{name:"kubeedge-controller-manager-webhook-preview",namespace:"kubeedge",path:$path,port:443},caBundle:$ca},
     rules:[{apiGroups:[$group],apiVersions:[$version],resources:[$resource],operations:$ops}]};
  {apiVersion:"admissionregistration.k8s.io/v1",kind:"ValidatingWebhookConfiguration",
   metadata:{name:"week5-business-validation"},webhooks:[
     hook("device.week5-business.kubeedge.io";"/devices";"devices.kubeedge.io";"v1beta1";"devices";["CREATE","UPDATE"]),
     hook("devicemodel.week5-business.kubeedge.io";"/devicemodels";"devices.kubeedge.io";"v1beta1";"devicemodels";["CREATE","UPDATE"]),
     hook("rule.week5-business.kubeedge.io";"/rules";"rules.kubeedge.io";"v1";"rules";["CREATE","UPDATE","DELETE"]),
     hook("ruleendpoint.week5-business.kubeedge.io";"/ruleendpoints";"rules.kubeedge.io";"v1";"ruleendpoints";["CREATE","UPDATE","DELETE"]),
     hook("nodeupgradejob.week5-business.kubeedge.io";"/nodeupgradejobs";"operations.kubeedge.io";"v1alpha1";"nodeupgradejobs";["CREATE","UPDATE"])
   ]}' | k apply -f -

jq -n --arg ca "$ca_b64" '
  def hook($name; $path; $group; $version; $resource; $labels):
    {name:$name,admissionReviewVersions:["v1"],sideEffects:"None",failurePolicy:"Fail",timeoutSeconds:5,
     objectSelector:{matchLabels:$labels},
     clientConfig:{service:{name:"kubeedge-controller-manager-webhook-preview",namespace:"kubeedge",path:$path,port:443},caBundle:$ca},
     rules:[{apiGroups:[$group],apiVersions:[$version],resources:[$resource],operations:["CREATE","UPDATE"]}]};
  {apiVersion:"admissionregistration.k8s.io/v1",kind:"MutatingWebhookConfiguration",
   metadata:{name:"week5-business-mutation"},webhooks:[
     hook("offlinemigration.week5-business.kubeedge.io";"/offlinemigration";"";"v1";"pods";{"week5.kubeedge.io/business-test":"true","app-offline.kubeedge.io":"autonomy"}),
     hook("nodeupgradejob-mutation.week5-business.kubeedge.io";"/mutating/nodeupgradejobs";"operations.kubeedge.io";"v1alpha1";"nodeupgradejobs";{"week5.kubeedge.io/business-test":"true"})
   ]}' | k apply -f -

validating=$(k get validatingwebhookconfiguration week5-business-validation -o json | jq '.webhooks | length')
mutating=$(k get mutatingwebhookconfiguration week5-business-mutation -o json | jq '.webhooks | length')
[[ "$validating" == 5 && "$mutating" == 2 ]] || {
  echo "Expected 5 validating and 2 mutating webhooks; got $validating and $mutating" >&2
  exit 1
}

echo "Registered $validating validating and $mutating mutating webhooks in context $context"
k get validatingwebhookconfiguration week5-business-validation -o json | jq -r '.webhooks[].name'
k get mutatingwebhookconfiguration week5-business-mutation -o json | jq -r '.webhooks[].name'
