FROM alpine:3.20

# 参数 cfssl版本
ARG CFSSL_VERSION=1.6.5
ARG CFSSLJSON_VERSION=1.6.5

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
RUN chmod +x /usr/local/bin/cfssl /usr/local/bin/cfssljson

COPY ar /ar
