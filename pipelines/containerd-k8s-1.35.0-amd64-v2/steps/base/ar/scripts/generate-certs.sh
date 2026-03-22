#!/bin/bash
set -euo pipefail

# ============================================================================
# generate-certs.sh - 生成 K8s 所有组件的证书、kubeconfig 和 bootstrap 配置
# ============================================================================
# 用法: bash generate-certs.sh <master_ips> <etcd_ips>
# 例:   bash generate-certs.sh "172.19.16.11" "172.19.16.11"
# ============================================================================

die() { echo "ERROR: $*" >&2; exit 1; }

MASTER_IPS="${1:?用法: $0 <master_ips> <etcd_ips>}"
ETCD_IPS="${2:?用法: $0 <master_ips> <etcd_ips>}"

PKI_DIR="/ar-data/pki"
PEM_DIR="/ar/scripts/gen-pem"
CA_CONFIG="${PEM_DIR}/ca-config.json"
CA_PEM="${PKI_DIR}/ca.pem"
CA_KEY="${PKI_DIR}/ca-key.pem"

# 前置检查
[[ -f "$CA_PEM" ]]    || die "CA 证书不存在: $CA_PEM"
[[ -f "$CA_KEY" ]]    || die "CA 密钥不存在: $CA_KEY"
[[ -f "$CA_CONFIG" ]] || die "CA 配置不存在: $CA_CONFIG"

chmod a+x /usr/local/bin/cfssl /usr/local/bin/cfssljson
mkdir -p "$PKI_DIR"
cd "$PEM_DIR"

# --- 1. apiserver 证书 ---
APISERVER_SAN="kubernetes,kubernetes.default,kubernetes.default.svc,kubernetes.default.svc.cluster.local"
APISERVER_SAN="${APISERVER_SAN},127.0.0.1,10.96.0.1,10.103.97.12"
APISERVER_SAN="${APISERVER_SAN},${MASTER_IPS}"

echo "生成 apiserver 证书, SAN: ${APISERVER_SAN}"
cfssl gencert \
  -ca="$CA_PEM" \
  -ca-key="$CA_KEY" \
  -config="$CA_CONFIG" \
  -hostname="${APISERVER_SAN}" \
  -profile=kubernetes \
  apiserver-csr.json | cfssljson -bare "${PKI_DIR}/apiserver"

# --- 2. controller-manager 证书 ---
echo "生成 controller-manager 证书"
cfssl gencert \
  -ca="$CA_PEM" \
  -ca-key="$CA_KEY" \
  -config="$CA_CONFIG" \
  -profile=kubernetes \
  controller-manager-csr.json | cfssljson -bare "${PKI_DIR}/controller-manager"

# --- 3. scheduler 证书 ---
echo "生成 scheduler 证书"
cfssl gencert \
  -ca="$CA_PEM" \
  -ca-key="$CA_KEY" \
  -config="$CA_CONFIG" \
  -profile=kubernetes \
  scheduler-csr.json | cfssljson -bare "${PKI_DIR}/scheduler"

# --- 4. admin 证书 ---
echo "生成 admin 证书"
cfssl gencert \
  -ca="$CA_PEM" \
  -ca-key="$CA_KEY" \
  -config="$CA_CONFIG" \
  -profile=kubernetes \
  admin-csr.json | cfssljson -bare "${PKI_DIR}/admin"

# --- 5. kube-proxy 证书 ---
echo "生成 kube-proxy 证书"
cfssl gencert \
  -ca="$CA_PEM" \
  -ca-key="$CA_KEY" \
  -config="$CA_CONFIG" \
  -profile=kubernetes \
  kube-proxy-csr.json | cfssljson -bare "${PKI_DIR}/kube-proxy"

# --- 6. front-proxy-ca (独立 CA) ---
echo "生成 front-proxy-ca"
cfssl gencert -initca front-proxy-ca-csr.json | cfssljson -bare "${PKI_DIR}/front-proxy-ca"

# --- 7. front-proxy-client 证书 ---
echo "生成 front-proxy-client 证书"
cfssl gencert \
  -ca="${PKI_DIR}/front-proxy-ca.pem" \
  -ca-key="${PKI_DIR}/front-proxy-ca-key.pem" \
  -config="$CA_CONFIG" \
  -profile=kubernetes \
  front-proxy-client-csr.json | cfssljson -bare "${PKI_DIR}/front-proxy-client"

# --- 8. ServiceAccount 密钥对 ---
echo "生成 ServiceAccount 密钥对"
openssl genrsa -out "${PKI_DIR}/sa.key" 2048 2>/dev/null
openssl rsa -in "${PKI_DIR}/sa.key" -pubout -out "${PKI_DIR}/sa.pub" 2>/dev/null

# --- 9. 生成 bootstrap token ---
TOKEN_ID="$(head -c 6 /dev/urandom | od -An -tx1 | tr -d ' \n' | head -c 6)"
TOKEN_SECRET="$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n' | head -c 16)"
echo "${TOKEN_ID}" > "${PKI_DIR}/bootstrap-token-id"
echo "${TOKEN_SECRET}" > "${PKI_DIR}/bootstrap-token-secret"
BOOTSTRAP_TOKEN="${TOKEN_ID}.${TOKEN_SECRET}"
echo "Bootstrap Token: ${BOOTSTRAP_TOKEN}"

# --- 10. 解压 kubectl 用于生成 kubeconfig ---
if ! command -v kubectl >/dev/null 2>&1; then
  echo "解压 kubectl..."
  tar -xf /ar/tar/kubernetes-server-linux-amd64.tar.gz \
    --strip-components=3 -C /usr/local/bin/ \
    kubernetes/server/bin/kubectl
  chmod +x /usr/local/bin/kubectl
fi

# --- 11. 生成 kubeconfig 文件 ---
APISERVER_URL="https://10.103.97.12:6443"
KUBECONFIG_DIR="/ar-data/kubeconfig"
mkdir -p "$KUBECONFIG_DIR"

# admin.kubeconfig
kubectl config set-cluster kubernetes \
  --certificate-authority="${CA_PEM}" \
  --embed-certs=true \
  --server="${APISERVER_URL}" \
  --kubeconfig="${KUBECONFIG_DIR}/admin.kubeconfig" >/dev/null
kubectl config set-credentials admin \
  --client-certificate="${PKI_DIR}/admin.pem" \
  --client-key="${PKI_DIR}/admin-key.pem" \
  --embed-certs=true \
  --kubeconfig="${KUBECONFIG_DIR}/admin.kubeconfig" >/dev/null
kubectl config set-context admin@kubernetes \
  --cluster=kubernetes --user=admin \
  --kubeconfig="${KUBECONFIG_DIR}/admin.kubeconfig" >/dev/null
kubectl config use-context admin@kubernetes \
  --kubeconfig="${KUBECONFIG_DIR}/admin.kubeconfig" >/dev/null

# controller-manager.kubeconfig
kubectl config set-cluster kubernetes \
  --certificate-authority="${CA_PEM}" \
  --embed-certs=true \
  --server="${APISERVER_URL}" \
  --kubeconfig="${KUBECONFIG_DIR}/controller-manager.kubeconfig" >/dev/null
kubectl config set-credentials system:kube-controller-manager \
  --client-certificate="${PKI_DIR}/controller-manager.pem" \
  --client-key="${PKI_DIR}/controller-manager-key.pem" \
  --embed-certs=true \
  --kubeconfig="${KUBECONFIG_DIR}/controller-manager.kubeconfig" >/dev/null
kubectl config set-context system:kube-controller-manager@kubernetes \
  --cluster=kubernetes --user=system:kube-controller-manager \
  --kubeconfig="${KUBECONFIG_DIR}/controller-manager.kubeconfig" >/dev/null
kubectl config use-context system:kube-controller-manager@kubernetes \
  --kubeconfig="${KUBECONFIG_DIR}/controller-manager.kubeconfig" >/dev/null

# scheduler.kubeconfig
kubectl config set-cluster kubernetes \
  --certificate-authority="${CA_PEM}" \
  --embed-certs=true \
  --server="${APISERVER_URL}" \
  --kubeconfig="${KUBECONFIG_DIR}/scheduler.kubeconfig" >/dev/null
kubectl config set-credentials system:kube-scheduler \
  --client-certificate="${PKI_DIR}/scheduler.pem" \
  --client-key="${PKI_DIR}/scheduler-key.pem" \
  --embed-certs=true \
  --kubeconfig="${KUBECONFIG_DIR}/scheduler.kubeconfig" >/dev/null
kubectl config set-context system:kube-scheduler@kubernetes \
  --cluster=kubernetes --user=system:kube-scheduler \
  --kubeconfig="${KUBECONFIG_DIR}/scheduler.kubeconfig" >/dev/null
kubectl config use-context system:kube-scheduler@kubernetes \
  --kubeconfig="${KUBECONFIG_DIR}/scheduler.kubeconfig" >/dev/null

# kube-proxy.kubeconfig
kubectl config set-cluster kubernetes \
  --certificate-authority="${CA_PEM}" \
  --embed-certs=true \
  --server="${APISERVER_URL}" \
  --kubeconfig="${KUBECONFIG_DIR}/kube-proxy.kubeconfig" >/dev/null
kubectl config set-credentials system:kube-proxy \
  --client-certificate="${PKI_DIR}/kube-proxy.pem" \
  --client-key="${PKI_DIR}/kube-proxy-key.pem" \
  --embed-certs=true \
  --kubeconfig="${KUBECONFIG_DIR}/kube-proxy.kubeconfig" >/dev/null
kubectl config set-context system:kube-proxy@kubernetes \
  --cluster=kubernetes --user=system:kube-proxy \
  --kubeconfig="${KUBECONFIG_DIR}/kube-proxy.kubeconfig" >/dev/null
kubectl config use-context system:kube-proxy@kubernetes \
  --kubeconfig="${KUBECONFIG_DIR}/kube-proxy.kubeconfig" >/dev/null

# bootstrap-kubelet.kubeconfig
kubectl config set-cluster kubernetes \
  --certificate-authority="${CA_PEM}" \
  --embed-certs=true \
  --server="${APISERVER_URL}" \
  --kubeconfig="${KUBECONFIG_DIR}/bootstrap-kubelet.kubeconfig" >/dev/null
kubectl config set-credentials tls-bootstrap-token-user \
  --token="${BOOTSTRAP_TOKEN}" \
  --kubeconfig="${KUBECONFIG_DIR}/bootstrap-kubelet.kubeconfig" >/dev/null
kubectl config set-context tls-bootstrap-token-user@kubernetes \
  --cluster=kubernetes --user=tls-bootstrap-token-user \
  --kubeconfig="${KUBECONFIG_DIR}/bootstrap-kubelet.kubeconfig" >/dev/null
kubectl config use-context tls-bootstrap-token-user@kubernetes \
  --kubeconfig="${KUBECONFIG_DIR}/bootstrap-kubelet.kubeconfig" >/dev/null

# --- 12. 生成 bootstrap RBAC YAML ---
cat > "/ar-data/bootstrap-secret.yaml" <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: bootstrap-token-${TOKEN_ID}
  namespace: kube-system
type: bootstrap.kubernetes.io/token
stringData:
  token-id: "${TOKEN_ID}"
  token-secret: "${TOKEN_SECRET}"
  usage-bootstrap-authentication: "true"
  usage-bootstrap-signing: "true"
  auth-extra-groups: system:bootstrappers:default-node-token
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubelet-bootstrap
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:node-bootstrapper
subjects:
  - apiGroup: rbac.authorization.k8s.io
    kind: Group
    name: system:bootstrappers:default-node-token
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: node-autoapprove-bootstrap
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:certificates.k8s.io:certificatesigningrequests:nodeclient
subjects:
  - apiGroup: rbac.authorization.k8s.io
    kind: Group
    name: system:bootstrappers:default-node-token
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: node-autoapprove-certificate-rotation
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:certificates.k8s.io:certificatesigningrequests:selfnodeclient
subjects:
  - apiGroup: rbac.authorization.k8s.io
    kind: Group
    name: system:nodes
EOF

echo "所有证书、kubeconfig 和 bootstrap 配置生成完成"
ls -la "${PKI_DIR}/"
ls -la "${KUBECONFIG_DIR}/"
