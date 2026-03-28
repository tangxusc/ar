FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    ca-certificates \
    coreutils \
    openssh-client \
    rsync \
    sshpass \
    && rm -rf /var/lib/apt/lists/*

COPY ar /ar
