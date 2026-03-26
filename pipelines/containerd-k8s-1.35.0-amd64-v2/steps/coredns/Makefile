SHELL := /bin/bash

IMAGE_NAME := containerd-k8s-1_35_0-coredns:latest
IMAGE_STORE_DIR := /var/lib/ar/images/containerd-k8s-1_35_0-coredns
THIS_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
DOCKERFILE := $(THIS_DIR)/Dockerfile

.PHONY: build offline-images

COREDNS_REF := registry.k8s.io/coredns/coredns:v1.14.2
COREDNS_ARCHIVE := $(THIS_DIR)/ar/images/registry.k8s.io-coredns-coredns-v1.14.2.tar

offline-images:
	mkdir -p "$(THIS_DIR)/ar/images"
	@if [ ! -s "$(COREDNS_ARCHIVE)" ]; then \
		echo "Downloading offline image: $(COREDNS_REF) -> $(COREDNS_ARCHIVE)"; \
		skopeo copy --override-os linux --override-arch amd64 "docker://$(COREDNS_REF)" "oci-archive:$(COREDNS_ARCHIVE):$(COREDNS_REF)"; \
	else \
		echo "Offline image exists, skip: $(COREDNS_ARCHIVE)"; \
	fi

build:
	$(MAKE) offline-images
	docker build -f "$(DOCKERFILE)" -t $(IMAGE_NAME) .
	rm -rf "$(IMAGE_STORE_DIR)"
	skopeo copy docker-daemon:$(IMAGE_NAME) oci:$(IMAGE_STORE_DIR):latest
	allrun image list
