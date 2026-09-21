#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail

namespace=${1:-kubeedge}
output_dir=${2:-./webhook-certs}
service_name=kubeedge-admission-service

mkdir -p "${output_dir}"
chmod 700 "${output_dir}"
config_file=$(mktemp)
trap 'rm -f "${config_file}"' EXIT

cat >"${config_file}" <<EOF
[req]
distinguished_name = subject
x509_extensions = extensions
prompt = no
[subject]
CN = ${service_name}.${namespace}.svc
[extensions]
subjectAltName = @alt_names
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
[alt_names]
DNS.1 = ${service_name}
DNS.2 = ${service_name}.${namespace}
DNS.3 = ${service_name}.${namespace}.svc
DNS.4 = ${service_name}.${namespace}.svc.cluster.local
EOF

openssl genrsa -out "${output_dir}/ca.key" 4096
openssl req -x509 -new -sha256 -key "${output_dir}/ca.key" -days 3650 \
  -subj "/CN=kubeedge-webhook-ca" -out "${output_dir}/ca.crt"
openssl genrsa -out "${output_dir}/tls.key" 2048
openssl req -new -key "${output_dir}/tls.key" -config "${config_file}" \
  -out "${output_dir}/tls.csr"
openssl x509 -req -sha256 -in "${output_dir}/tls.csr" \
  -CA "${output_dir}/ca.crt" -CAkey "${output_dir}/ca.key" -CAcreateserial \
  -days 365 -extensions extensions -extfile "${config_file}" \
  -out "${output_dir}/tls.crt"

rm -f "${output_dir}/tls.csr" "${output_dir}/ca.srl"
chmod 600 "${output_dir}/ca.key" "${output_dir}/tls.key"
chmod 644 "${output_dir}/ca.crt" "${output_dir}/tls.crt"

if base64 --help 2>&1 | grep -q -- '-w'; then
  base64 -w0 "${output_dir}/ca.crt" >"${output_dir}/ca-bundle.txt"
else
  base64 <"${output_dir}/ca.crt" | tr -d '\n' >"${output_dir}/ca-bundle.txt"
fi

cat <<EOF
Generated webhook certificates in ${output_dir}.

Create or replace the TLS Secret:
  kubectl -n ${namespace} create secret tls kubeedge-webhook-certs \\
    --cert=${output_dir}/tls.crt \\
    --key=${output_dir}/tls.key \\
    --dry-run=client -o yaml | kubectl apply -f -

The CA bundle is available in ${output_dir}/ca-bundle.txt.
EOF
