#!/usr/bin/env bash
set -euo pipefail

build_nodes_report() {
  local i
  for i in "${!NODE_IPS[@]}"; do
    printf '%s\t%s\t%s\t%s\n' "${NODE_IPS[$i]}" "${NODE_PORTS[$i]}" "$(role_of "${NODE_LABELS[$i]}")" "${NODE_LABELS[$i]}"
  done
}

action_print_plan() {
  mkdir -p /tasks/cluster /tasks/inventory /tasks/reports
  cat > /tasks/cluster/metadata.json <<EOF
{
  "pipelineName": "${PIPELINE_NAME:-containerd-k8s-1_35_0-amd64}",
  "containerdVersion": "${CONTAINERD_VERSION}",
  "kubernetesVersion": "${KUBERNETES_SERVER_VERSION}",
  "ciliumCliVersion": "${CILIUM_CLI_VERSION}",
  "lvscareVersion": "${LVSCARE_VERSION}"
}
EOF
  printf '%s\n' "${NODES_MATRIX}" > /tasks/inventory/nodes.matrix
  build_nodes_report > /tasks/reports/plan.txt
  write_current_summary "Printed execution plan"
}

action_validate_inventory() {
  local masters=0 workers=0 i
  for i in "${!NODE_IPS[@]}"; do
    case "$(role_of "${NODE_LABELS[$i]}")" in
      master) ((masters+=1)) ;;
      worker) ((workers+=1)) ;;
    esac
  done
  if [[ "${masters}" -lt 1 ]]; then
    echo "no master nodes found in labels" > /current-task/error.txt
    exit 1
  fi
  if [[ "${workers}" -lt 1 ]]; then
    echo "no worker nodes found in labels" > /current-task/error.txt
    exit 1
  fi
  {
    echo "masters=${masters}"
    echo "workers=${workers}"
  } > /tasks/reports/inventory-validation.txt
  write_current_summary "Validated node inventory"
}

action_precheck_all() {
  local i
  for i in "${!NODE_IPS[@]}"; do
    remote_exec "${i}" "uname -a && hostname && df -h / && free -m" > "/tasks/reports/precheck-${NODE_IPS[$i]}.txt"
  done
  write_current_summary "Ran precheck on all nodes"
}

action_os_baseline_all() {
  local i
  for i in "${!NODE_IPS[@]}"; do
    remote_exec "${i}" "sudo mkdir -p /etc/modules-load.d /etc/sysctl.d ${PIPELINE_WORK_DIR} /var/lib/registry && sudo swapoff -a || true && printf 'overlay\nbr_netfilter\n' | sudo tee /etc/modules-load.d/containerd.conf >/dev/null && printf 'net.bridge.bridge-nf-call-iptables = 1\nnet.ipv4.ip_forward = 1\nnet.bridge.bridge-nf-call-ip6tables = 1\n' | sudo tee /etc/sysctl.d/99-kubernetes-cri.conf >/dev/null && sudo sysctl --system >/dev/null"
    printf 'baseline-ok\n' > "/tasks/reports/os-baseline-${NODE_IPS[$i]}.txt"
  done
  write_current_summary "Applied baseline settings"
}

action_runtime_binaries_all() {
  local i
  for i in "${!NODE_IPS[@]}"; do
    remote_exec "${i}" "mkdir -p ${PIPELINE_WORK_DIR}/runtime ${PIPELINE_WORK_DIR}/images"
    remote_copy "${i}" "/artifacts/tarballs/containerd-${CONTAINERD_VERSION}-linux-amd64.tar.gz" "${PIPELINE_WORK_DIR}/runtime/"
    remote_copy "${i}" "/artifacts/tarballs/cni-plugins-linux-amd64-${CNI_PLUGINS_VERSION}.tgz" "${PIPELINE_WORK_DIR}/runtime/"
    remote_copy "${i}" "/artifacts/tarballs/crictl-${CRICTL_VERSION}-linux-amd64.tar.gz" "${PIPELINE_WORK_DIR}/runtime/"
    remote_copy "${i}" "/artifacts/tarballs/nerdctl-${NERDCTL_VERSION}-linux-amd64.tar.gz" "${PIPELINE_WORK_DIR}/runtime/"
    remote_copy "${i}" "/artifacts/bin/runc" "${PIPELINE_WORK_DIR}/runtime/runc"
    if label_has "${NODE_LABELS[$i]}" "registry" "true"; then
      if compgen -G "/artifacts/images/registry/*" > /dev/null; then
        for image_tar in /artifacts/images/registry/*; do
          remote_copy "${i}" "${image_tar}" "${PIPELINE_WORK_DIR}/images/"
        done
      fi
    fi
    printf 'runtime-copied\n' > "/tasks/reports/runtime-binaries-${NODE_IPS[$i]}.txt"
  done
  printf '%s\n' "containerd=${CONTAINERD_VERSION} cni=${CNI_PLUGINS_VERSION} crictl=${CRICTL_VERSION} nerdctl=${NERDCTL_VERSION}" > /tasks/reports/runtime-manifest.txt
  write_current_summary "Copied runtime artifacts to all nodes"
}

action_install_containerd_all() {
  local i endpoint
  endpoint="$(registry_endpoints)"
  export DEFAULT_REGISTRY_MIRRORS="${endpoint}"
  if [[ -n "${endpoint}" ]]; then
    export SANDBOX_IMAGE="${endpoint%%,*}/system/pause:3.10"
  else
    export SANDBOX_IMAGE="${PAUSE_IMAGE}"
  fi
  for i in "${!NODE_IPS[@]}"; do
    remote_exec "${i}" "sudo tar -C /usr/local -xzf ${PIPELINE_WORK_DIR}/runtime/containerd-${CONTAINERD_VERSION}-linux-amd64.tar.gz && sudo install -m 0755 ${PIPELINE_WORK_DIR}/runtime/runc /usr/local/sbin/runc && sudo mkdir -p /opt/cni/bin && sudo tar -C /opt/cni/bin -xzf ${PIPELINE_WORK_DIR}/runtime/cni-plugins-linux-amd64-${CNI_PLUGINS_VERSION}.tgz && sudo tar -C /usr/local/bin -xzf ${PIPELINE_WORK_DIR}/runtime/crictl-${CRICTL_VERSION}-linux-amd64.tar.gz && sudo tar -C /usr/local/bin -xzf ${PIPELINE_WORK_DIR}/runtime/nerdctl-${NERDCTL_VERSION}-linux-amd64.tar.gz"
    export NODE_IP="${NODE_IPS[$i]}"
    render_template "/templates/systemd/containerd.service" > /tmp/containerd.service
    render_template "/templates/config/containerd-config.toml.tmpl" > /tmp/config.toml
    remote_copy "${i}" "/tmp/containerd.service" "${PIPELINE_WORK_DIR}/containerd.service"
    remote_copy "${i}" "/tmp/config.toml" "${PIPELINE_WORK_DIR}/config.toml"
    remote_write "${i}" "/tmp/crictl.yaml" "$(cat /templates/config/crictl.yaml)"
    remote_exec "${i}" "sudo mkdir -p /etc/containerd /etc/containerd/certs.d/_default /etc/systemd/system && sudo cp ${PIPELINE_WORK_DIR}/containerd.service /etc/systemd/system/containerd.service && sudo cp ${PIPELINE_WORK_DIR}/config.toml /etc/containerd/config.toml && sudo cp /tmp/crictl.yaml /etc/crictl.yaml && sudo systemctl daemon-reload && sudo systemctl enable --now containerd"
    printf 'containerd-installed\n' > "/tasks/reports/containerd-install-${NODE_IPS[$i]}.txt"
  done
  rm -f /tmp/containerd.service /tmp/config.toml
  write_current_summary "Installed containerd on all nodes"
}

action_verify_containerd_all() {
  local i
  for i in "${!NODE_IPS[@]}"; do
    remote_exec "${i}" "systemctl is-active containerd && crictl info >/dev/null && nerdctl --version" > "/tasks/reports/containerd-verify-${NODE_IPS[$i]}.txt"
  done
  write_current_summary "Verified containerd on all nodes"
}

action_registry_install() {
  local idx image_tar
  while read -r idx; do
    [[ -z "${idx}" ]] && continue
    if ! compgen -G "/artifacts/images/registry/*" > /dev/null; then
      continue
    fi
    for image_tar in /artifacts/images/registry/*; do
      remote_copy "${idx}" "${image_tar}" "${PIPELINE_WORK_DIR}/images/"
      remote_exec "${idx}" "sudo ctr -n k8s.io images import ${PIPELINE_WORK_DIR}/images/$(basename "${image_tar}") || sudo ctr images import ${PIPELINE_WORK_DIR}/images/$(basename "${image_tar}")"
    done
    remote_exec "${idx}" "sudo mkdir -p /var/lib/registry && sudo nerdctl rm -f local-registry >/dev/null 2>&1 || true && sudo nerdctl run -d --restart always --net host -v /var/lib/registry:/var/lib/registry --name local-registry registry:2"
    printf 'registry-installed\n' > "/tasks/reports/registry-install-${NODE_IPS[$idx]}.txt"
  done < <(registry_indexes || true)
  write_current_summary "Installed local registry on registry nodes"
}

action_registry_push_images() {
  local targets target archive image_path archive_type repo image tag dest
  if [[ ! -f /metadata/manifest.json ]]; then
    write_current_summary "Skipped registry push because metadata/manifest.json is missing"
    return 0
  fi
  targets="$(registry_endpoints)"
  [[ -n "${targets}" ]] || {
    write_current_summary "Skipped registry push because no registry nodes were found"
    return 0
  }
  python3 - <<'PY' > /tmp/image-entries.txt
import json
from pathlib import Path
manifest = json.loads(Path("/metadata/manifest.json").read_text())
for item in manifest.get("images", []):
    archive_type = item.get("archive_type")
    target_repository = item.get("target_repository")
    target_image = item.get("target_image")
    target_tag = item.get("target_tag")
    path = item.get("path")
    if archive_type and target_repository and target_image and target_tag and path:
        if path.startswith("steps/base/artifacts/"):
            path = "/" + path.split("steps/base/", 1)[1]
        print(f"{archive_type}|{path}|{target_repository}|{target_image}|{target_tag}")
PY
  while IFS='|' read -r archive_type image_path repo image tag; do
    [[ -z "${archive_type}" ]] && continue
    for target in ${targets//,/ }; do
      dest="docker://${target#http://}/${repo}/${image}:${tag}"
      if [[ "${archive_type}" == "docker-archive" ]]; then
        skopeo copy "docker-archive:/${image_path}" "${dest}"
      elif [[ "${archive_type}" == "oci-archive" ]]; then
        skopeo copy "oci-archive:/${image_path}" "${dest}"
      fi
    done
  done < /tmp/image-entries.txt
  rm -f /tmp/image-entries.txt
  write_current_summary "Pushed offline images to all registry nodes"
}

action_configure_containerd_registry_all() {
  local idx endpoints
  endpoints="$(registry_endpoints)"
  for idx in "${!NODE_IPS[@]}"; do
    : > /tmp/hosts.toml
    for endpoint in ${endpoints//,/ }; do
      cat >> /tmp/hosts.toml <<EOF
[host."${endpoint}"]
  capabilities = ["pull", "resolve"]
  skip_verify = true

EOF
    done
    remote_copy "${idx}" /tmp/hosts.toml "${PIPELINE_WORK_DIR}/hosts.toml"
    remote_exec "${idx}" "sudo mkdir -p /etc/containerd/certs.d/_default && sudo cp ${PIPELINE_WORK_DIR}/hosts.toml /etc/containerd/certs.d/_default/hosts.toml && sudo systemctl restart containerd"
    cp /tmp/hosts.toml "/tasks/rendered/${NODE_IPS[$idx]}-hosts.toml"
    printf 'registry-configured\n' > "/tasks/reports/containerd-registry-${NODE_IPS[$idx]}.txt"
  done
  rm -f /tmp/hosts.toml
  write_current_summary "Configured containerd registry mirrors"
}

action_distribute_k8s_binaries() {
  local idx role
  for idx in "${!NODE_IPS[@]}"; do
    role="$(role_of "${NODE_LABELS[$idx]}")"
    remote_copy "${idx}" "/artifacts/tarballs/kubernetes-server-linux-amd64.tar.gz" "${PIPELINE_WORK_DIR}/"
    if [[ "${role}" == "master" ]]; then
      remote_copy "${idx}" "/artifacts/tarballs/etcd-${ETCD_VERSION}-linux-amd64.tar.gz" "${PIPELINE_WORK_DIR}/"
      remote_exec "${idx}" "sudo tar -xf ${PIPELINE_WORK_DIR}/kubernetes-server-linux-amd64.tar.gz --strip-components=3 -C /usr/local/bin kubernetes/server/bin/kube{let,ctl,-apiserver,-controller-manager,-scheduler,-proxy} && sudo tar -xf ${PIPELINE_WORK_DIR}/etcd-${ETCD_VERSION}-linux-amd64.tar.gz -C /tmp && sudo install -m 0755 /tmp/etcd-${ETCD_VERSION}-linux-amd64/etcd /usr/local/bin/etcd && sudo install -m 0755 /tmp/etcd-${ETCD_VERSION}-linux-amd64/etcdctl /usr/local/bin/etcdctl"
      printf 'master-binaries-installed\n' > "/tasks/reports/master-binaries-${NODE_IPS[$idx]}.txt"
    elif [[ "${role}" == "worker" ]]; then
      remote_exec "${idx}" "sudo tar -xf ${PIPELINE_WORK_DIR}/kubernetes-server-linux-amd64.tar.gz --strip-components=3 -C /usr/local/bin kubernetes/server/bin/kube{let,-proxy}"
      printf 'worker-binaries-installed\n' > "/tasks/reports/worker-binaries-${NODE_IPS[$idx]}.txt"
    fi
  done
  write_current_summary "Distributed Kubernetes binaries"
}

action_generate_pki() {
  local idx san_hosts token token_id token_secret
  idx="$(first_master_index)"
  mkdir -p /tasks/pki /tasks/kubeconfig /tasks/bootstrap
  remote_copy "${idx}" "/artifacts/bin/cfssl" "${PIPELINE_WORK_DIR}/cfssl"
  remote_copy "${idx}" "/artifacts/bin/cfssljson" "${PIPELINE_WORK_DIR}/cfssljson"
  remote_exec "${idx}" "chmod +x ${PIPELINE_WORK_DIR}/cfssl ${PIPELINE_WORK_DIR}/cfssljson && sudo mkdir -p /etc/kubernetes/pki /etc/etcd/ssl ${PIPELINE_WORK_DIR}/pki"
  remote_copy "${idx}" "/templates/pki/ca-config.json" "${PIPELINE_WORK_DIR}/pki/ca-config.json"
  remote_copy "${idx}" "/templates/pki/etcd-ca-csr.json" "${PIPELINE_WORK_DIR}/pki/etcd-ca-csr.json"
  remote_copy "${idx}" "/templates/pki/etcd-csr.json.tmpl" "${PIPELINE_WORK_DIR}/pki/etcd-csr.json"
  remote_copy "${idx}" "/templates/pki/kubernetes-ca-csr.json" "${PIPELINE_WORK_DIR}/pki/kubernetes-ca-csr.json"
  remote_copy "${idx}" "/templates/pki/apiserver-csr.json" "${PIPELINE_WORK_DIR}/pki/apiserver-csr.json"
  remote_copy "${idx}" "/templates/pki/front-proxy-ca-csr.json" "${PIPELINE_WORK_DIR}/pki/front-proxy-ca-csr.json"
  remote_copy "${idx}" "/templates/pki/front-proxy-client-csr.json" "${PIPELINE_WORK_DIR}/pki/front-proxy-client-csr.json"
  remote_copy "${idx}" "/templates/pki/controller-manager-csr.json" "${PIPELINE_WORK_DIR}/pki/controller-manager-csr.json"
  remote_copy "${idx}" "/templates/pki/scheduler-csr.json" "${PIPELINE_WORK_DIR}/pki/scheduler-csr.json"
  remote_copy "${idx}" "/templates/pki/admin-csr.json" "${PIPELINE_WORK_DIR}/pki/admin-csr.json"
  remote_copy "${idx}" "/templates/pki/kube-proxy-csr.json" "${PIPELINE_WORK_DIR}/pki/kube-proxy-csr.json"

  san_hosts="kubernetes,kubernetes.default,kubernetes.default.svc,kubernetes.default.svc.cluster.local,127.0.0.1,$(master_ips_csv),${NODE_IPS[$idx]}"
  remote_exec "${idx}" "cd ${PIPELINE_WORK_DIR}/pki && \
    ${PIPELINE_WORK_DIR}/cfssl gencert -initca kubernetes-ca-csr.json | ${PIPELINE_WORK_DIR}/cfssljson -bare ca >/dev/null && \
    sudo cp ca.pem ca-key.pem /etc/kubernetes/pki/ && \
    ${PIPELINE_WORK_DIR}/cfssl gencert -initca etcd-ca-csr.json | ${PIPELINE_WORK_DIR}/cfssljson -bare etcd-ca >/dev/null && \
    sudo cp etcd-ca.pem etcd-ca-key.pem /etc/etcd/ssl/ && \
    ${PIPELINE_WORK_DIR}/cfssl gencert -ca=etcd-ca.pem -ca-key=etcd-ca-key.pem -config=ca-config.json -profile=kubernetes -hostname=$(master_ips_csv),127.0.0.1 etcd-csr.json | ${PIPELINE_WORK_DIR}/cfssljson -bare etcd >/dev/null && \
    sudo cp etcd.pem etcd-key.pem /etc/etcd/ssl/ && \
    ${PIPELINE_WORK_DIR}/cfssl gencert -ca=ca.pem -ca-key=ca-key.pem -config=ca-config.json -profile=kubernetes -hostname=${san_hosts} apiserver-csr.json | ${PIPELINE_WORK_DIR}/cfssljson -bare apiserver >/dev/null && \
    sudo cp apiserver.pem apiserver-key.pem /etc/kubernetes/pki/ && \
    ${PIPELINE_WORK_DIR}/cfssl gencert -initca front-proxy-ca-csr.json | ${PIPELINE_WORK_DIR}/cfssljson -bare front-proxy-ca >/dev/null && \
    sudo cp front-proxy-ca.pem front-proxy-ca-key.pem /etc/kubernetes/pki/ && \
    ${PIPELINE_WORK_DIR}/cfssl gencert -ca=front-proxy-ca.pem -ca-key=front-proxy-ca-key.pem -config=ca-config.json -profile=kubernetes front-proxy-client-csr.json | ${PIPELINE_WORK_DIR}/cfssljson -bare front-proxy-client >/dev/null && \
    sudo cp front-proxy-client.pem front-proxy-client-key.pem /etc/kubernetes/pki/ && \
    ${PIPELINE_WORK_DIR}/cfssl gencert -ca=ca.pem -ca-key=ca-key.pem -config=ca-config.json -profile=kubernetes controller-manager-csr.json | ${PIPELINE_WORK_DIR}/cfssljson -bare controller-manager >/dev/null && \
    sudo cp controller-manager.pem controller-manager-key.pem /etc/kubernetes/pki/ && \
    ${PIPELINE_WORK_DIR}/cfssl gencert -ca=ca.pem -ca-key=ca-key.pem -config=ca-config.json -profile=kubernetes scheduler-csr.json | ${PIPELINE_WORK_DIR}/cfssljson -bare scheduler >/dev/null && \
    sudo cp scheduler.pem scheduler-key.pem /etc/kubernetes/pki/ && \
    ${PIPELINE_WORK_DIR}/cfssl gencert -ca=ca.pem -ca-key=ca-key.pem -config=ca-config.json -profile=kubernetes admin-csr.json | ${PIPELINE_WORK_DIR}/cfssljson -bare admin >/dev/null && \
    sudo cp admin.pem admin-key.pem /etc/kubernetes/pki/ && \
    ${PIPELINE_WORK_DIR}/cfssl gencert -ca=ca.pem -ca-key=ca-key.pem -config=ca-config.json -profile=kubernetes kube-proxy-csr.json | ${PIPELINE_WORK_DIR}/cfssljson -bare kube-proxy >/dev/null && \
    sudo cp kube-proxy.pem kube-proxy-key.pem /etc/kubernetes/pki/ && \
    openssl genrsa -out sa.key 2048 >/dev/null 2>&1 && openssl rsa -in sa.key -pubout -out sa.pub >/dev/null 2>&1 && sudo cp sa.key sa.pub /etc/kubernetes/pki/"

  remote_exec "${idx}" "kubectl config set-cluster kubernetes --certificate-authority=/etc/kubernetes/pki/ca.pem --embed-certs=true --server=https://${NODE_IPS[$idx]}:6443 --kubeconfig=/etc/kubernetes/admin.kubeconfig >/dev/null && \
    kubectl config set-credentials admin --client-certificate=/etc/kubernetes/pki/admin.pem --client-key=/etc/kubernetes/pki/admin-key.pem --embed-certs=true --kubeconfig=/etc/kubernetes/admin.kubeconfig >/dev/null && \
    kubectl config set-context admin@kubernetes --cluster=kubernetes --user=admin --kubeconfig=/etc/kubernetes/admin.kubeconfig >/dev/null && \
    kubectl config use-context admin@kubernetes --kubeconfig=/etc/kubernetes/admin.kubeconfig >/dev/null && \
    sudo mkdir -p /root/.kube && sudo cp /etc/kubernetes/admin.kubeconfig /root/.kube/config"
  remote_exec "${idx}" "kubectl config set-cluster kubernetes --certificate-authority=/etc/kubernetes/pki/ca.pem --embed-certs=true --server=https://${NODE_IPS[$idx]}:6443 --kubeconfig=/etc/kubernetes/controller-manager.kubeconfig >/dev/null && \
    kubectl config set-credentials system:kube-controller-manager --client-certificate=/etc/kubernetes/pki/controller-manager.pem --client-key=/etc/kubernetes/pki/controller-manager-key.pem --embed-certs=true --kubeconfig=/etc/kubernetes/controller-manager.kubeconfig >/dev/null && \
    kubectl config set-context system:kube-controller-manager@kubernetes --cluster=kubernetes --user=system:kube-controller-manager --kubeconfig=/etc/kubernetes/controller-manager.kubeconfig >/dev/null && \
    kubectl config use-context system:kube-controller-manager@kubernetes --kubeconfig=/etc/kubernetes/controller-manager.kubeconfig >/dev/null"
  remote_exec "${idx}" "kubectl config set-cluster kubernetes --certificate-authority=/etc/kubernetes/pki/ca.pem --embed-certs=true --server=https://${NODE_IPS[$idx]}:6443 --kubeconfig=/etc/kubernetes/scheduler.kubeconfig >/dev/null && \
    kubectl config set-credentials system:kube-scheduler --client-certificate=/etc/kubernetes/pki/scheduler.pem --client-key=/etc/kubernetes/pki/scheduler-key.pem --embed-certs=true --kubeconfig=/etc/kubernetes/scheduler.kubeconfig >/dev/null && \
    kubectl config set-context system:kube-scheduler@kubernetes --cluster=kubernetes --user=system:kube-scheduler --kubeconfig=/etc/kubernetes/scheduler.kubeconfig >/dev/null && \
    kubectl config use-context system:kube-scheduler@kubernetes --kubeconfig=/etc/kubernetes/scheduler.kubeconfig >/dev/null"
  remote_exec "${idx}" "kubectl config set-cluster kubernetes --certificate-authority=/etc/kubernetes/pki/ca.pem --embed-certs=true --server=https://${NODE_IPS[$idx]}:6443 --kubeconfig=/etc/kubernetes/kube-proxy.kubeconfig >/dev/null && \
    kubectl config set-credentials system:kube-proxy --client-certificate=/etc/kubernetes/pki/kube-proxy.pem --client-key=/etc/kubernetes/pki/kube-proxy-key.pem --embed-certs=true --kubeconfig=/etc/kubernetes/kube-proxy.kubeconfig >/dev/null && \
    kubectl config set-context system:kube-proxy@kubernetes --cluster=kubernetes --user=system:kube-proxy --kubeconfig=/etc/kubernetes/kube-proxy.kubeconfig >/dev/null && \
    kubectl config use-context system:kube-proxy@kubernetes --kubeconfig=/etc/kubernetes/kube-proxy.kubeconfig >/dev/null"

  token_id="$(head -c 6 /dev/urandom | md5sum | head -c 6)"
  token_secret="$(head -c 16 /dev/urandom | md5sum | head -c 16)"
  token="${token_id}.${token_secret}"
  remote_exec "${idx}" "kubectl config set-cluster kubernetes --certificate-authority=/etc/kubernetes/pki/ca.pem --embed-certs=true --server=https://${NODE_IPS[$idx]}:6443 --kubeconfig=/etc/kubernetes/bootstrap-kubelet.kubeconfig >/dev/null && \
    kubectl config set-credentials tls-bootstrap-token-user --token=${token} --kubeconfig=/etc/kubernetes/bootstrap-kubelet.kubeconfig >/dev/null && \
    kubectl config set-context tls-bootstrap-token-user@kubernetes --cluster=kubernetes --user=tls-bootstrap-token-user --kubeconfig=/etc/kubernetes/bootstrap-kubelet.kubeconfig >/dev/null && \
    kubectl config use-context tls-bootstrap-token-user@kubernetes --kubeconfig=/etc/kubernetes/bootstrap-kubelet.kubeconfig >/dev/null"

  remote_fetch "${idx}" "/etc/kubernetes/pki/ca.pem" "/tasks/pki/ca.pem"
  remote_fetch "${idx}" "/etc/kubernetes/pki/ca-key.pem" "/tasks/pki/ca-key.pem"
  remote_fetch "${idx}" "/etc/kubernetes/pki/apiserver.pem" "/tasks/pki/apiserver.pem"
  remote_fetch "${idx}" "/etc/kubernetes/pki/apiserver-key.pem" "/tasks/pki/apiserver-key.pem"
  remote_fetch "${idx}" "/etc/kubernetes/pki/front-proxy-ca.pem" "/tasks/pki/front-proxy-ca.pem"
  remote_fetch "${idx}" "/etc/kubernetes/pki/front-proxy-client.pem" "/tasks/pki/front-proxy-client.pem"
  remote_fetch "${idx}" "/etc/kubernetes/pki/front-proxy-client-key.pem" "/tasks/pki/front-proxy-client-key.pem"
  remote_fetch "${idx}" "/etc/kubernetes/pki/controller-manager.pem" "/tasks/pki/controller-manager.pem"
  remote_fetch "${idx}" "/etc/kubernetes/pki/controller-manager-key.pem" "/tasks/pki/controller-manager-key.pem"
  remote_fetch "${idx}" "/etc/kubernetes/pki/scheduler.pem" "/tasks/pki/scheduler.pem"
  remote_fetch "${idx}" "/etc/kubernetes/pki/scheduler-key.pem" "/tasks/pki/scheduler-key.pem"
  remote_fetch "${idx}" "/etc/kubernetes/pki/admin.pem" "/tasks/pki/admin.pem"
  remote_fetch "${idx}" "/etc/kubernetes/pki/admin-key.pem" "/tasks/pki/admin-key.pem"
  remote_fetch "${idx}" "/etc/kubernetes/pki/kube-proxy.pem" "/tasks/pki/kube-proxy.pem"
  remote_fetch "${idx}" "/etc/kubernetes/pki/kube-proxy-key.pem" "/tasks/pki/kube-proxy-key.pem"
  remote_fetch "${idx}" "/etc/kubernetes/pki/sa.key" "/tasks/pki/sa.key"
  remote_fetch "${idx}" "/etc/kubernetes/pki/sa.pub" "/tasks/pki/sa.pub"
  remote_fetch "${idx}" "/etc/etcd/ssl/etcd-ca.pem" "/tasks/pki/etcd-ca.pem"
  remote_fetch "${idx}" "/etc/etcd/ssl/etcd-ca-key.pem" "/tasks/pki/etcd-ca-key.pem"
  remote_fetch "${idx}" "/etc/etcd/ssl/etcd.pem" "/tasks/pki/etcd.pem"
  remote_fetch "${idx}" "/etc/etcd/ssl/etcd-key.pem" "/tasks/pki/etcd-key.pem"
  remote_fetch "${idx}" "/etc/kubernetes/admin.kubeconfig" "/tasks/kubeconfig/admin.kubeconfig"
  remote_fetch "${idx}" "/etc/kubernetes/controller-manager.kubeconfig" "/tasks/kubeconfig/controller-manager.kubeconfig"
  remote_fetch "${idx}" "/etc/kubernetes/scheduler.kubeconfig" "/tasks/kubeconfig/scheduler.kubeconfig"
  remote_fetch "${idx}" "/etc/kubernetes/kube-proxy.kubeconfig" "/tasks/kubeconfig/kube-proxy.kubeconfig"
  remote_fetch "${idx}" "/etc/kubernetes/bootstrap-kubelet.kubeconfig" "/tasks/kubeconfig/bootstrap-kubelet.kubeconfig"

  export BOOTSTRAP_TOKEN_ID="${token_id}"
  export BOOTSTRAP_TOKEN_SECRET="${token_secret}"
  render_template "/templates/config/bootstrap.secret.yaml.tmpl" > /tasks/bootstrap/bootstrap.secret.yaml
  printf 'pki-generated\n' > /tasks/reports/pki-generation.txt
  write_current_summary "Generated PKI assets on first master"
}

action_distribute_pki() {
  local idx role
  for idx in "${!NODE_IPS[@]}"; do
    role="$(role_of "${NODE_LABELS[$idx]}")"
    remote_exec "${idx}" "sudo mkdir -p /etc/kubernetes/pki /etc/etcd/ssl"
    remote_copy "${idx}" "/tasks/pki/ca.pem" "/tmp/ca.pem"
    remote_copy "${idx}" "/tasks/pki/front-proxy-ca.pem" "/tmp/front-proxy-ca.pem"
    remote_exec "${idx}" "sudo cp /tmp/ca.pem /etc/kubernetes/pki/ca.pem && sudo cp /tmp/front-proxy-ca.pem /etc/kubernetes/pki/front-proxy-ca.pem"
    if [[ "${role}" == "master" ]]; then
      for f in apiserver.pem apiserver-key.pem front-proxy-client.pem front-proxy-client-key.pem controller-manager.pem controller-manager-key.pem scheduler.pem scheduler-key.pem admin.pem admin-key.pem kube-proxy.pem kube-proxy-key.pem sa.key sa.pub; do
        remote_copy "${idx}" "/tasks/pki/${f}" "/tmp/${f}"
      done
      for f in etcd-ca.pem etcd-ca-key.pem etcd.pem etcd-key.pem; do
        remote_copy "${idx}" "/tasks/pki/${f}" "/tmp/${f}"
      done
      remote_copy "${idx}" "/tasks/kubeconfig/admin.kubeconfig" "/tmp/admin.kubeconfig"
      remote_copy "${idx}" "/tasks/kubeconfig/controller-manager.kubeconfig" "/tmp/controller-manager.kubeconfig"
      remote_copy "${idx}" "/tasks/kubeconfig/scheduler.kubeconfig" "/tmp/scheduler.kubeconfig"
      remote_copy "${idx}" "/tasks/kubeconfig/kube-proxy.kubeconfig" "/tmp/kube-proxy.kubeconfig"
      remote_copy "${idx}" "/tasks/kubeconfig/bootstrap-kubelet.kubeconfig" "/tmp/bootstrap-kubelet.kubeconfig"
      remote_exec "${idx}" "sudo cp /tmp/apiserver.pem /tmp/apiserver-key.pem /tmp/front-proxy-client.pem /tmp/front-proxy-client-key.pem /tmp/controller-manager.pem /tmp/controller-manager-key.pem /tmp/scheduler.pem /tmp/scheduler-key.pem /tmp/admin.pem /tmp/admin-key.pem /tmp/kube-proxy.pem /tmp/kube-proxy-key.pem /tmp/sa.key /tmp/sa.pub /etc/kubernetes/pki/ && sudo cp /tmp/etcd-ca.pem /tmp/etcd-ca-key.pem /tmp/etcd.pem /tmp/etcd-key.pem /etc/etcd/ssl/ && sudo cp /tmp/admin.kubeconfig /tmp/controller-manager.kubeconfig /tmp/scheduler.kubeconfig /tmp/kube-proxy.kubeconfig /tmp/bootstrap-kubelet.kubeconfig /etc/kubernetes/"
      printf 'master-pki-distributed\n' > "/tasks/reports/master-pki-distribution-${NODE_IPS[$idx]}.txt"
    elif [[ "${role}" == "worker" ]]; then
      remote_copy "${idx}" "/tasks/kubeconfig/kube-proxy.kubeconfig" "/tmp/kube-proxy.kubeconfig"
      remote_copy "${idx}" "/tasks/kubeconfig/bootstrap-kubelet.kubeconfig" "/tmp/bootstrap-kubelet.kubeconfig"
      remote_exec "${idx}" "sudo cp /tmp/kube-proxy.kubeconfig /tmp/bootstrap-kubelet.kubeconfig /etc/kubernetes/"
      printf 'worker-pki-distributed\n' > "/tasks/reports/worker-pki-distribution-${NODE_IPS[$idx]}.txt"
    fi
  done
  write_current_summary "Distributed PKI assets to cluster nodes"
}

action_install_etcd() {
  local idx initial_cluster parts=()
  for idx in "${!NODE_IPS[@]}"; do
    if [[ "$(role_of "${NODE_LABELS[$idx]}")" == "master" ]]; then
      parts+=("${NODE_IPS[$idx]}=https://${NODE_IPS[$idx]}:2380")
    fi
  done
  initial_cluster="$(IFS=,; echo "${parts[*]}")"
  for idx in "${!NODE_IPS[@]}"; do
    if [[ "$(role_of "${NODE_LABELS[$idx]}")" != "master" ]]; then
      continue
    fi
    export NODE_IP="${NODE_IPS[$idx]}"
    export ETCD_INITIAL_CLUSTER="${initial_cluster}"
    render_template "/templates/config/etcd.config.yml.tmpl" > "/tasks/rendered/${NODE_IPS[$idx]}-etcd.config.yml"
    remote_copy "${idx}" "/tasks/rendered/${NODE_IPS[$idx]}-etcd.config.yml" "/tmp/etcd.config.yml"
    remote_copy "${idx}" "/templates/systemd/etcd.service" "/tmp/etcd.service"
    remote_exec "${idx}" "sudo mkdir -p /etc/etcd /var/lib/etcd /etc/systemd/system && sudo cp /tmp/etcd.config.yml /etc/etcd/etcd.config.yml && sudo cp /tmp/etcd.service /etc/systemd/system/etcd.service && sudo systemctl daemon-reload && sudo systemctl enable --now etcd"
    printf 'etcd-installed\n' > "/tasks/reports/etcd-install-${NODE_IPS[$idx]}.txt"
  done
  write_current_summary "Installed etcd on master nodes"
}

action_verify_etcd() {
  local idx master_idx endpoints=()
  master_idx="$(first_master_index)"
  for idx in "${!NODE_IPS[@]}"; do
    if [[ "$(role_of "${NODE_LABELS[$idx]}")" == "master" ]]; then
      endpoints+=("https://${NODE_IPS[$idx]}:2379")
    fi
  done
  remote_exec "${master_idx}" "ETCDCTL_API=3 etcdctl --endpoints=$(IFS=,; echo "${endpoints[*]}") --cacert=/etc/etcd/ssl/etcd-ca.pem --cert=/etc/etcd/ssl/etcd.pem --key=/etc/etcd/ssl/etcd-key.pem endpoint status --write-out=table" > /tasks/reports/etcd-endpoint-status.txt
  write_current_summary "Verified etcd cluster"
}

action_render_ha() {
  local idx master_idx j registry_repo
  master_idx="$(first_master_index)"
  export HA_ENDPOINT="https://${NODE_IPS[$master_idx]}:6443"
  export MASTER_RS_ARGS=""
  registry_repo="$(registry_endpoints)"
  registry_repo="${registry_repo%%,*}"
  registry_repo="${registry_repo#http://}"
  if [[ -n "${registry_repo}" ]]; then
    export LVSCARE_POD_IMAGE="${registry_repo}/system/pause:3.10"
  else
    export LVSCARE_POD_IMAGE="${PAUSE_IMAGE}"
  fi
  for j in "${!NODE_IPS[@]}"; do
    if [[ "$(role_of "${NODE_LABELS[$j]}")" == "master" ]]; then
      MASTER_RS_ARGS+=$'        - --rs\n'
      MASTER_RS_ARGS+="        - ${NODE_IPS[$j]}:6443"$'\n'
    fi
  done
  for idx in "${!NODE_IPS[@]}"; do
    if [[ "$(role_of "${NODE_LABELS[$idx]}")" != "master" ]]; then
      continue
    fi
    render_template "/templates/manifests/lvscare-static-pod.yaml.tmpl" > "/tasks/rendered/${NODE_IPS[$idx]}-lvscare.yaml"
    printf 'LVSCARE_VS=%s:6443\n' "${NODE_IPS[$master_idx]}" > "/tasks/rendered/${NODE_IPS[$idx]}-lvscare.env"
  done
  write_current_summary "Rendered lvs-care manifests"
}

action_install_ha() {
  local idx
  while read -r idx; do
    [[ -z "${idx}" ]] && continue
    if [[ "$(role_of "${NODE_LABELS[$idx]}")" != "master" ]]; then
      continue
    fi
    if [[ -f "/tasks/rendered/${NODE_IPS[$idx]}-lvscare.yaml" ]]; then
      remote_copy "${idx}" "/tasks/rendered/${NODE_IPS[$idx]}-lvscare.yaml" "/tmp/lvscare.yaml"
      remote_exec "${idx}" "sudo mkdir -p /etc/kubernetes/manifests && sudo cp /tmp/lvscare.yaml /etc/kubernetes/manifests/lvscare.yaml"
    fi
  done < <(seq 0 $((${#NODE_IPS[@]} - 1)))
  write_current_summary "Installed lvs-care manifests on master nodes"
}

action_install_apiserver() {
  local idx endpoints=()
  for idx in "${!NODE_IPS[@]}"; do
    if [[ "$(role_of "${NODE_LABELS[$idx]}")" == "master" ]]; then
      endpoints+=("https://${NODE_IPS[$idx]}:2379")
    fi
  done
  local etcd_servers
  etcd_servers="$(IFS=,; echo "${endpoints[*]}")"
  export SERVICE_CIDR="${SERVICE_CIDR:-10.96.0.0/12}"
  for idx in "${!NODE_IPS[@]}"; do
    if [[ "$(role_of "${NODE_LABELS[$idx]}")" != "master" ]]; then
      continue
    fi
    export NODE_IP="${NODE_IPS[$idx]}"
    export ETCD_SERVERS="${etcd_servers}"
    render_template "/templates/systemd/kube-apiserver.service.tmpl" > "/tasks/rendered/${NODE_IPS[$idx]}-kube-apiserver.service"
    remote_copy "${idx}" "/tasks/rendered/${NODE_IPS[$idx]}-kube-apiserver.service" "/tmp/kube-apiserver.service"
    remote_exec "${idx}" "sudo cp /tmp/kube-apiserver.service /etc/systemd/system/kube-apiserver.service && sudo systemctl daemon-reload && sudo systemctl enable --now kube-apiserver && systemctl is-active kube-apiserver >/dev/null && curl -k --max-time 10 https://127.0.0.1:6443/readyz >/dev/null"
    printf 'apiserver-installed\n' > "/tasks/reports/apiserver-install-${NODE_IPS[$idx]}.txt"
  done
  write_current_summary "Installed kube-apiserver on master nodes"
}

action_install_controller_manager() {
  local idx
  export SERVICE_CIDR="${SERVICE_CIDR:-10.96.0.0/12}"
  export POD_CIDR="${POD_CIDR:-10.244.0.0/16}"
  for idx in "${!NODE_IPS[@]}"; do
    if [[ "$(role_of "${NODE_LABELS[$idx]}")" != "master" ]]; then
      continue
    fi
    render_template "/templates/systemd/kube-controller-manager.service.tmpl" > "/tasks/rendered/${NODE_IPS[$idx]}-kube-controller-manager.service"
    remote_copy "${idx}" "/tasks/rendered/${NODE_IPS[$idx]}-kube-controller-manager.service" "/tmp/kube-controller-manager.service"
    remote_exec "${idx}" "sudo cp /tmp/kube-controller-manager.service /etc/systemd/system/kube-controller-manager.service && sudo systemctl daemon-reload && sudo systemctl enable --now kube-controller-manager && systemctl is-active kube-controller-manager >/dev/null"
    printf 'controller-manager-installed\n' > "/tasks/reports/controller-manager-install-${NODE_IPS[$idx]}.txt"
  done
  write_current_summary "Installed kube-controller-manager on master nodes"
}

action_install_scheduler() {
  local idx
  for idx in "${!NODE_IPS[@]}"; do
    if [[ "$(role_of "${NODE_LABELS[$idx]}")" != "master" ]]; then
      continue
    fi
    remote_copy "${idx}" "/templates/systemd/kube-scheduler.service.tmpl" "/tmp/kube-scheduler.service"
    remote_exec "${idx}" "sudo cp /tmp/kube-scheduler.service /etc/systemd/system/kube-scheduler.service && sudo systemctl daemon-reload && sudo systemctl enable --now kube-scheduler && systemctl is-active kube-scheduler >/dev/null"
    printf 'scheduler-installed\n' > "/tasks/reports/scheduler-install-${NODE_IPS[$idx]}.txt"
  done
  write_current_summary "Installed kube-scheduler on master nodes"
}

action_apply_bootstrap_rbac() {
  local idx
  idx="$(first_master_index)"
  remote_copy "${idx}" "/tasks/bootstrap/bootstrap.secret.yaml" "/tmp/bootstrap.secret.yaml"
  remote_exec "${idx}" "KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl apply -f /tmp/bootstrap.secret.yaml || true"
  printf 'bootstrap-rbac-applied\n' > /tasks/reports/bootstrap-rbac.txt
  write_current_summary "Applied bootstrap RBAC manifest"
}

action_render_kubelet_all() {
  local idx
  for idx in "${!NODE_IPS[@]}"; do
    export NODE_IP="${NODE_IPS[$idx]}"
    render_template "/templates/systemd/kubelet.service.tmpl" > "/tasks/rendered/${NODE_IPS[$idx]}-kubelet.service"
    render_template "/templates/config/kubelet-conf.yml.tmpl" > "/tasks/rendered/${NODE_IPS[$idx]}-kubelet-conf.yml"
  done
  write_current_summary "Rendered kubelet configs for all nodes"
}

action_install_kubelet_master() {
  local idx
  for idx in "${!NODE_IPS[@]}"; do
    if [[ "$(role_of "${NODE_LABELS[$idx]}")" != "master" ]]; then
      continue
    fi
    remote_copy "${idx}" "/tasks/rendered/${NODE_IPS[$idx]}-kubelet.service" "/tmp/kubelet.service"
    remote_copy "${idx}" "/tasks/rendered/${NODE_IPS[$idx]}-kubelet-conf.yml" "/tmp/kubelet-conf.yml"
    remote_exec "${idx}" "sudo mkdir -p /etc/kubernetes /etc/systemd/system /var/lib/kubelet /etc/kubernetes/manifests && sudo cp /tmp/kubelet.service /etc/systemd/system/kubelet.service && sudo cp /tmp/kubelet-conf.yml /etc/kubernetes/kubelet-conf.yml && sudo systemctl daemon-reload && sudo systemctl enable --now kubelet"
    printf 'kubelet-master-installed\n' > "/tasks/reports/kubelet-install-${NODE_IPS[$idx]}.txt"
  done
  write_current_summary "Installed kubelet on master nodes"
}

action_verify_ha() {
  local idx ha_endpoint
  idx="$(first_master_index)"
  ha_endpoint="https://${NODE_IPS[$idx]}:6443"
  {
    remote_exec "${idx}" "test -f /etc/kubernetes/manifests/lvscare.yaml && systemctl is-active kubelet && crictl ps | grep -E 'lvscare|pause' && ipvsadm -Ln"
    remote_exec "${idx}" "curl -k --max-time 10 ${ha_endpoint}/healthz"
    remote_exec "${idx}" "curl -k --max-time 10 ${ha_endpoint}/readyz"
  } > /tasks/reports/ha-verify.txt
  write_current_summary "Verified HA state and VIP readiness"
}

action_install_kubelet_worker() {
  local idx
  for idx in "${!NODE_IPS[@]}"; do
    if [[ "$(role_of "${NODE_LABELS[$idx]}")" != "worker" ]]; then
      continue
    fi
    remote_copy "${idx}" "/tasks/rendered/${NODE_IPS[$idx]}-kubelet.service" "/tmp/kubelet.service"
    remote_copy "${idx}" "/tasks/rendered/${NODE_IPS[$idx]}-kubelet-conf.yml" "/tmp/kubelet-conf.yml"
    remote_exec "${idx}" "sudo mkdir -p /etc/kubernetes /etc/systemd/system /var/lib/kubelet && sudo cp /tmp/kubelet.service /etc/systemd/system/kubelet.service && sudo cp /tmp/kubelet-conf.yml /etc/kubernetes/kubelet-conf.yml && sudo systemctl daemon-reload && sudo systemctl enable --now kubelet"
    printf 'kubelet-worker-installed\n' >> "/tasks/reports/kubelet-install-${NODE_IPS[$idx]}.txt"
  done
  write_current_summary "Installed kubelet on worker nodes"
}

action_verify_node_bootstrap() {
  local idx expected_count
  idx="$(first_master_index)"
  expected_count="${#NODE_IPS[@]}"
  remote_exec "${idx}" "KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl get nodes -o wide" > /tasks/reports/node-bootstrap.txt
  remote_exec "${idx}" "count=\$(KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl get nodes --no-headers | wc -l); [[ \"\${count}\" -eq ${expected_count} ]]" 
  remote_exec "${idx}" "! KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl get nodes --no-headers | awk '\$2 !~ /^Ready/ {print}' | grep -q ."
  write_current_summary "Verified node bootstrap and Ready status"
}

action_install_kube_proxy() {
  local idx
  export POD_CIDR="${POD_CIDR:-10.244.0.0/16}"
  for idx in "${!NODE_IPS[@]}"; do
    remote_copy "${idx}" "/templates/systemd/kube-proxy.service.tmpl" "/tmp/kube-proxy.service"
    render_template "/templates/config/kube-proxy.yaml.tmpl" > /tmp/kube-proxy.yaml
    remote_copy "${idx}" "/tmp/kube-proxy.yaml" "/tmp/kube-proxy.yaml"
    remote_exec "${idx}" "sudo cp /tmp/kube-proxy.service /etc/systemd/system/kube-proxy.service && sudo cp /tmp/kube-proxy.yaml /etc/kubernetes/kube-proxy.yaml && sudo systemctl daemon-reload && sudo systemctl enable --now kube-proxy"
    printf 'kube-proxy-installed\n' > "/tasks/reports/kube-proxy-install-${NODE_IPS[$idx]}.txt"
  done
  rm -f /tmp/kube-proxy.yaml
  write_current_summary "Installed kube-proxy on all nodes"
}

action_install_cni() {
  local idx registry_repo
  idx="$(first_master_index)"
  remote_copy "${idx}" "/artifacts/tarballs/cilium-linux-amd64.tar.gz" "${PIPELINE_WORK_DIR}/cilium-linux-amd64.tar.gz"
  remote_copy "${idx}" "/artifacts/manifests/cilium/install.env" "${PIPELINE_WORK_DIR}/install.env"
  remote_copy "${idx}" "/artifacts/tarballs/cilium-chart-${CILIUM_VERSION}.tar.gz" "${PIPELINE_WORK_DIR}/cilium-chart-${CILIUM_VERSION}.tar.gz" || true
  remote_exec "${idx}" "sudo tar -C /usr/local/bin -xzf ${PIPELINE_WORK_DIR}/cilium-linux-amd64.tar.gz && mkdir -p ${PIPELINE_WORK_DIR}/cilium-chart && tar -xf ${PIPELINE_WORK_DIR}/cilium-chart-${CILIUM_VERSION}.tar.gz -C ${PIPELINE_WORK_DIR} >/dev/null 2>&1 || true"
  cp /artifacts/manifests/cilium/install.env /tasks/rendered/cilium-install.env
  registry_repo="$(registry_endpoints)"
  registry_repo="${registry_repo%%,*}"
  registry_repo="${registry_repo#http://}"
  [[ -n "${registry_repo}" ]] || {
    echo "no registry endpoints available for offline Cilium install" > /current-task/error.txt
    exit 1
  }
  remote_exec "${idx}" "source ${PIPELINE_WORK_DIR}/install.env && cilium install --chart-directory ${PIPELINE_WORK_DIR}/cilium-${CILIUM_VERSION}/install/kubernetes/cilium --version ${CILIUM_VERSION} --wait --set image.repository=${registry_repo}/cilium/cilium --set image.tag=${CILIUM_VERSION} --set operator.image.repository=${registry_repo}/cilium/operator --set operator.image.tag=${CILIUM_VERSION} --set hubble.relay.image.repository=${registry_repo}/cilium/hubble-relay --set hubble.relay.image.tag=${CILIUM_VERSION} --set hubble.ui.backend.image.repository=${registry_repo}/cilium/hubble-ui-backend --set hubble.ui.backend.image.tag=${CILIUM_VERSION} --set hubble.ui.frontend.image.repository=${registry_repo}/cilium/hubble-ui --set hubble.ui.frontend.image.tag=${CILIUM_VERSION} --set-string securityContext.capabilities.ciliumAgent='{CHOWN,KILL,NET_ADMIN,NET_RAW,IPC_LOCK,SYS_ADMIN,SYS_RESOURCE,DAC_OVERRIDE,FOWNER,SETGID,SETUID}' --set cni.exclusive=false --set-string extraConfig.prepend-iptables-chains=false" > /tasks/reports/cni-install.txt
  remote_exec "${idx}" "cilium status --wait" >> /tasks/reports/cni-install.txt
  printf 'cilium chart and images prepared via local registry mirror\n' > /tasks/reports/cilium-images-import.txt
  write_current_summary "Installed or prepared Cilium on first master"
}

action_install_coredns() {
  local idx registry_repo
  idx="$(first_master_index)"
  registry_repo="$(registry_endpoints)"
  registry_repo="${registry_repo%%,*}"
  registry_repo="${registry_repo#http://}"
  [[ -n "${registry_repo}" ]] || {
    echo "no registry endpoints available for offline CoreDNS install" > /current-task/error.txt
    exit 1
  }
  remote_copy "${idx}" "/artifacts/tarballs/helm-v${HELM_VERSION}-linux-amd64.tar.gz" "${PIPELINE_WORK_DIR}/helm-v${HELM_VERSION}.tar.gz"
  remote_copy "${idx}" "/artifacts/tarballs/coredns-chart-${COREDNS_CHART_VERSION}.tar.gz" "${PIPELINE_WORK_DIR}/coredns-chart-${COREDNS_CHART_VERSION}.tar.gz"
  remote_exec "${idx}" "mkdir -p ${PIPELINE_WORK_DIR}/helm && tar -xf ${PIPELINE_WORK_DIR}/helm-v${HELM_VERSION}.tar.gz -C ${PIPELINE_WORK_DIR}/helm && sudo install -m 0755 ${PIPELINE_WORK_DIR}/helm/linux-amd64/helm /usr/local/bin/helm"
  remote_exec "${idx}" "tar -xf ${PIPELINE_WORK_DIR}/coredns-chart-${COREDNS_CHART_VERSION}.tar.gz -C ${PIPELINE_WORK_DIR}"
  remote_copy "${idx}" "/artifacts/manifests/coredns/values.yaml" "${PIPELINE_WORK_DIR}/coredns-values.yaml"
  remote_exec "${idx}" "python3 - <<'PY'\nfrom pathlib import Path\np = Path('${PIPELINE_WORK_DIR}/coredns-values.yaml')\ntext = p.read_text()\ntext = text.replace('coredns/coredns', '${registry_repo}/dns/coredns')\np.write_text(text)\nPY"
  remote_exec "${idx}" "helm upgrade --install coredns ${PIPELINE_WORK_DIR}/helm-coredns-${COREDNS_CHART_VERSION}/charts/coredns -n kube-system --create-namespace -f ${PIPELINE_WORK_DIR}/coredns-values.yaml --set service.clusterIP=10.96.0.10 --set image.repository=${registry_repo}/dns/coredns --set image.tag=${COREDNS_IMAGE_TAG} --wait" > /tasks/reports/coredns-install.txt
  printf 'coredns images should be available in registry mirrors\n' > /tasks/reports/coredns-images-import.txt
  remote_fetch "${idx}" "${PIPELINE_WORK_DIR}/coredns-values.yaml" "/tasks/rendered/coredns-values.yaml"
  write_current_summary "Installed or prepared CoreDNS on first master"
}

action_verify_cluster_final() {
  local idx expected_total
  idx="$(first_master_index)"
  expected_total="${#NODE_IPS[@]}"
  remote_exec "${idx}" "KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl get nodes --no-headers >/dev/null"
  remote_exec "${idx}" "! KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl get nodes --no-headers | awk '\$2 !~ /^Ready/ {print}' | grep -q ."
  remote_exec "${idx}" "count=\$(KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl get nodes --no-headers | wc -l); [[ \${count} -eq ${expected_total} ]]"
  remote_exec "${idx}" "KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl -n kube-system get ds cilium >/dev/null"
  remote_exec "${idx}" "KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl -n kube-system rollout status ds/cilium --timeout=120s"
  remote_exec "${idx}" "desired=\$(KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl -n kube-system get ds cilium -o jsonpath='{.status.desiredNumberScheduled}'); ready=\$(KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl -n kube-system get ds cilium -o jsonpath='{.status.numberReady}'); [[ \${desired} = \${ready} ]]"
  remote_exec "${idx}" "KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl -n kube-system get deploy coredns >/dev/null"
  remote_exec "${idx}" "KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl -n kube-system rollout status deploy/coredns --timeout=120s"
  remote_exec "${idx}" "desired=\$(KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl -n kube-system get deploy coredns -o jsonpath='{.status.replicas}'); ready=\$(KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl -n kube-system get deploy coredns -o jsonpath='{.status.readyReplicas}'); [[ -n \${desired} && \${desired} = \${ready} ]]"
  {
    echo "== nodes =="
    remote_exec "${idx}" "KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl get nodes -o wide"
    echo "== pods =="
    remote_exec "${idx}" "KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl get pods -A"
    echo "== services =="
    remote_exec "${idx}" "KUBECONFIG=/etc/kubernetes/admin.kubeconfig kubectl get svc -A"
  } > /tasks/reports/final-summary.txt
  printf '{"status":"success"}\n' > /tasks/reports/final-summary.json
  write_current_summary "Verified final cluster state"
}

dispatch_action() {
  case "$1" in
    print-plan) action_print_plan ;;
    validate-inventory) action_validate_inventory ;;
    precheck-all) action_precheck_all ;;
    os-baseline-all) action_os_baseline_all ;;
    runtime-binaries-all) action_runtime_binaries_all ;;
    install-containerd-all) action_install_containerd_all ;;
    verify-containerd-all) action_verify_containerd_all ;;
    registry-install) action_registry_install ;;
    registry-push-images) action_registry_push_images ;;
    configure-containerd-registry-all) action_configure_containerd_registry_all ;;
    distribute-k8s-binaries) action_distribute_k8s_binaries ;;
    generate-pki) action_generate_pki ;;
    distribute-pki) action_distribute_pki ;;
    install-etcd) action_install_etcd ;;
    verify-etcd) action_verify_etcd ;;
    render-ha) action_render_ha ;;
    install-ha) action_install_ha ;;
    install-apiserver) action_install_apiserver ;;
    install-controller-manager) action_install_controller_manager ;;
    install-scheduler) action_install_scheduler ;;
    apply-bootstrap-rbac) action_apply_bootstrap_rbac ;;
    render-kubelet-all) action_render_kubelet_all ;;
    install-kubelet-master) action_install_kubelet_master ;;
    verify-ha) action_verify_ha ;;
    install-kubelet-worker) action_install_kubelet_worker ;;
    verify-node-bootstrap) action_verify_node_bootstrap ;;
    install-kube-proxy) action_install_kube_proxy ;;
    install-cni) action_install_cni ;;
    install-coredns) action_install_coredns ;;
    verify-cluster-final) action_verify_cluster_final ;;
    *)
      echo "unknown action: $1" >&2
      exit 1
      ;;
  esac
}
