FROM ubuntu:22.04

# 参数 cfssl版本
ARG CFSSL_VERSION=1.6.5
ARG CFSSLJSON_VERSION=1.6.5
#
ARG CONTAINERD_VERSION=2.2.1
ARG CNI_PLUGINS_VERSION=1.9.0
ARG CRICTL_VERSION=1.35.0
ARG NERDCTL_VERSION=2.2.1
ARG RUNC_VERSION=1.2.6
# k8s版本
ARG KUBERNETES_SERVER_VERSION=1.35.0
ARG KUBECTL_VERSION=v1.35.0
ARG CLI_ARCH=amd64

# etcd版本
ARG ETCD_VERSION=3.6.8

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    ca-certificates \
    coreutils \
    curl \
    gettext-base \
    jq \
    net-tools \
    openssh-client \
    openssl \
    rsync \
    skopeo \
    sshpass \
    tar \
    wget \
    && rm -rf /var/lib/apt/lists/*

# 安装 yq
RUN wget "https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64" -O /usr/local/bin/yq && chmod +x /usr/local/bin/yq

# 下载cfssl 和cfssljson
RUN wget "https://github.com/cloudflare/cfssl/releases/download/v${CFSSL_VERSION}/cfssl_${CFSSL_VERSION}_linux_amd64" -O /usr/local/bin/cfssl
RUN wget "https://github.com/cloudflare/cfssl/releases/download/v${CFSSL_VERSION}/cfssljson_${CFSSL_VERSION}_linux_amd64" -O /usr/local/bin/cfssljson
RUN chmod a+x /usr/local/bin/cfssl /usr/local/bin/cfssljson

COPY ar /ar

RUN mkdir -p /ar/tar

# 下载containerd 和cni-plugins 和crictl 和nerdctl
RUN wget "https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-${CONTAINERD_VERSION}-linux-amd64.tar.gz" -O /ar/tar/containerd-${CONTAINERD_VERSION}-linux-amd64.tar.gz
# https://github.com/containernetworking/plugins/releases/download/v1.9.0/cni-plugins-linux-amd64-v1.9.0.tgz
RUN wget "https://github.com/containernetworking/plugins/releases/download/v${CNI_PLUGINS_VERSION}/cni-plugins-linux-amd64-v${CNI_PLUGINS_VERSION}.tgz" -O /ar/tar/cni-plugins-linux-amd64-v${CNI_PLUGINS_VERSION}.tgz
# https://github.com/kubernetes-sigs/cri-tools/releases/download/v1.35.0/crictl-v1.35.0-linux-amd64.tar.gz
RUN wget "https://github.com/kubernetes-sigs/cri-tools/releases/download/v${CRICTL_VERSION}/crictl-v${CRICTL_VERSION}-linux-amd64.tar.gz" -O /ar/tar/crictl-v${CRICTL_VERSION}-linux-amd64.tar.gz
# https://github.com/containerd/nerdctl/releases/download/v2.2.1/nerdctl-2.2.1-linux-amd64.tar.gz
RUN wget "https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VERSION}/nerdctl-${NERDCTL_VERSION}-linux-amd64.tar.gz" -O /ar/tar/nerdctl-${NERDCTL_VERSION}-linux-amd64.tar.gz
# https://github.com/opencontainers/runc/releases/download/v1.2.6/runc.amd64
RUN wget "https://github.com/opencontainers/runc/releases/download/v${RUNC_VERSION}/runc.amd64" -O /ar/tar/runc.amd64 && chmod 0755 /ar/tar/runc.amd64

# 下载etcd
RUN wget "https://github.com/etcd-io/etcd/releases/download/v${ETCD_VERSION}/etcd-v${ETCD_VERSION}-linux-amd64.tar.gz" -O /ar/tar/etcd-v${ETCD_VERSION}-linux-amd64.tar.gz

# 下载k8s
RUN wget "https://dl.k8s.io/release/v${KUBERNETES_SERVER_VERSION}/kubernetes-server-linux-amd64.tar.gz" -O /ar/tar/kubernetes-server-linux-amd64.tar.gz
# 下载kubectl
RUN curl -LO https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${CLI_ARCH}/kubectl && mv kubectl /usr/bin/ && chmod +x /usr/bin/kubectl

