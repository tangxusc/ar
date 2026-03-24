FROM alpine:3.23.3

ENV HELM_VERSION=v4.1.3
ENV KUBECTL_VERSION=v1.35.0
ENV CILIUM_CLI_VERSION=v0.19.2
ENV CLI_ARCH=amd64
ENV KUBECONFIG=/ar-data/kubeconfig/master.kubeconfig

RUN apk add --no-cache \
    bash \
    curl \
    net-tools \
    rsync \
    skopeo \
    sshpass \
    tar \
    wget

RUN mkdir -p /ar/
WORKDIR /ar
RUN wget -O helm.tar.gz https://get.helm.sh/helm-${HELM_VERSION}-linux-${CLI_ARCH}.tar.gz && tar -xzf helm.tar.gz && mv linux-${CLI_ARCH}/helm /usr/local/bin/ && chmod +x /usr/local/bin/helm && rm helm.tar.gz
RUN curl -LO https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${CLI_ARCH}/kubectl && mv kubectl /usr/local/bin/ && chmod +x /usr/local/bin/kubectl 
RUN curl -L --fail --remote-name-all https://github.com/cilium/cilium-cli/releases/download/${CILIUM_CLI_VERSION}/cilium-linux-${CLI_ARCH}.tar.gz && tar xzvfC cilium-linux-${CLI_ARCH}.tar.gz /usr/local/bin && rm cilium-linux-${CLI_ARCH}.tar.gz



