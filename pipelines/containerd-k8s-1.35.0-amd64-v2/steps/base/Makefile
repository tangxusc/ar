SHELL := /bin/bash

IMAGE_NAME := containerd-k8s-1_35_0-base:latest
THIS_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
PIPELINE_ROOT := $(abspath $(THIS_DIR)/../..)
DOCKERFILE := $(THIS_DIR)/Dockerfile
TMP_IMAGE_TAR := /tmp/containerd-k8s-1_35_0-base.tar.gz
TMP_IMAGE_DIR := /tmp/containerd-k8s-1_35_0-base

.PHONY: build

build:
	docker build -f "$(DOCKERFILE)" -t $(IMAGE_NAME) .
	rm -rf "$(TMP_IMAGE_TAR)" "$(TMP_IMAGE_DIR)"
	docker save $(IMAGE_NAME) -o "$(TMP_IMAGE_TAR)"
	mkdir -p "$(TMP_IMAGE_DIR)"
	tar -xf "$(TMP_IMAGE_TAR)" -C "$(TMP_IMAGE_DIR)"
	rm -rf /var/lib/ar/images/containerd-k8s-1_35_0-base
	cp -r "$(TMP_IMAGE_DIR)" /var/lib/ar/images/containerd-k8s-1_35_0-base
	rm -rf "$(TMP_IMAGE_TAR)" "$(TMP_IMAGE_DIR)"
	allrun image list