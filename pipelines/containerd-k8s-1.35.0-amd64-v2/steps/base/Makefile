SHELL := /bin/bash

IMAGE_NAME := containerd-k8s-1_35_0-base:latest
IMAGE_STORE_DIR := /var/lib/ar/images/containerd-k8s-1_35_0-base
THIS_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
PIPELINE_ROOT := $(abspath $(THIS_DIR)/../..)
DOCKERFILE := $(THIS_DIR)/Dockerfile

.PHONY: build offline-images

REGISTRY_REF := docker.io/library/registry:2.8.3
REGISTRY_ARCHIVE := $(THIS_DIR)/ar/images/registry-2.8.3.tar

offline-images:
	mkdir -p "$(THIS_DIR)/ar/images"
	@if [ ! -s "$(REGISTRY_ARCHIVE)" ]; then \
		echo "Downloading offline image: $(REGISTRY_REF) -> $(REGISTRY_ARCHIVE)"; \
		skopeo copy --override-os linux --override-arch amd64 "docker://$(REGISTRY_REF)" "oci-archive:$(REGISTRY_ARCHIVE):registry:2.8.3"; \
	else \
		echo "Offline image exists, skip: $(REGISTRY_ARCHIVE)"; \
	fi

build:
	$(MAKE) offline-images
	docker build -f "$(DOCKERFILE)" -t $(IMAGE_NAME) .
	rm -rf "$(IMAGE_STORE_DIR)"
	skopeo copy docker-daemon:$(IMAGE_NAME) oci:$(IMAGE_STORE_DIR):latest
	allrun image list
