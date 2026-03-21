SHELL := /bin/bash

IMAGE_NAME := containerd-k8s-1_35_0-base:latest
IMAGE_STORE_DIR := /var/lib/ar/images/containerd-k8s-1_35_0-base
THIS_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
PIPELINE_ROOT := $(abspath $(THIS_DIR)/../..)
DOCKERFILE := $(THIS_DIR)/Dockerfile

.PHONY: build

build:
	docker build -f "$(DOCKERFILE)" -t $(IMAGE_NAME) .
	rm -rf "$(IMAGE_STORE_DIR)"
	skopeo copy docker-daemon:$(IMAGE_NAME) oci:$(IMAGE_STORE_DIR):latest
	allrun image list