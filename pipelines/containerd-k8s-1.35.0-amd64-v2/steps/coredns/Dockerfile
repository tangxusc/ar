FROM alpine:3.23.3

ENV KUBECTL_VERSION=v1.35.0
ENV CLI_ARCH=amd64
ENV KUBECONFIG=/ar-data/kubeconfig/master.kubeconfig

RUN apk add --no-cache \
    bash \
    curl \
    openssh-client \
    rsync \
    skopeo \
    sshpass

COPY ./ar/ /ar/
WORKDIR /ar

RUN curl -LO https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${CLI_ARCH}/kubectl \
    && mv kubectl /usr/local/bin/kubectl \
    && chmod +x /usr/local/bin/kubectl
