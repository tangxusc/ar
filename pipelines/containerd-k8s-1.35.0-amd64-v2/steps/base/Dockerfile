FROM alpine:3.20

# 参数 cfssl版本
ARG CFSSL_VERSION=1.6.5
ARG CFSSLJSON_VERSION=1.6.5
# 
ARG CONTAINERD_VERSION=2.2.1
ARG CNI_PLUGINS_VERSION=1.9.0
ARG CRICTL_VERSION=1.35.0
ARG NERDCTL_VERSION=2.2.1
# k8s版本
ARG KUBERNETES_SERVER_VERSION=1.35.0

# etcd版本
ARG ETCD_VERSION=3.6.8

RUN apk add --no-cache \
    bash \
    ca-certificates \
    coreutils \
    curl \
    gettext-envsubst \
    jq \
    net-tools \
    openssh-client \
    rsync \
    skopeo \
    sshpass \
    tar \
    wget \
    yq 
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

# 下载etcd
RUN wget "https://github.com/etcd-io/etcd/releases/download/v${ETCD_VERSION}/etcd-v${ETCD_VERSION}-linux-amd64.tar.gz" -O /ar/tar/etcd-v${ETCD_VERSION}-linux-amd64.tar.gz

# 下载k8s
# RUN wget "https://dl.k8s.io/release/v${KUBERNETES_SERVER_VERSION}/kubernetes-server-linux-amd64.tar.gz" -O /ar/tar/kubernetes-server-linux-amd64.tar.gz