FROM alpine:3.20

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

COPY scripts /scripts
COPY templates /templates
COPY metadata /metadata
COPY steps/base/artifacts /artifacts

RUN chmod a+x /scripts/remote/*.sh /scripts/download/*.sh /scripts/package/*.sh