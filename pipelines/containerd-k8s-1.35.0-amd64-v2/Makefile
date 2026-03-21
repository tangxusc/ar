SHELL := /bin/bash

PIPELINE_NAME := containerd-k8s-1_35_0-amd64-v2
PIPELINE_TEMPLATE := containerd-k8s-1_35_0-amd64-v2.template.json
PIPELINE_IMAGE := pipeline-$(PIPELINE_NAME):latest
PIPELINE_ROOT := $(CURDIR)
NODE_FILE ?= /home/ubuntu/ar/backend/test/node.json

.PHONY: help clean clean-package prepare-dirs download-artifacts download-runtime-artifacts download-kubernetes-artifacts download-pki-artifacts download-cni-artifacts download-addon-artifacts verify-artifacts package-artifacts build-step-images install-local-pipeline build-pipeline-image build dist test

help:
	@echo "Targets:"
	@echo "  prepare-dirs                Create artifact directories"
	@echo "  download-artifacts          Download all offline artifacts"
	@echo "  verify-artifacts            Validate artifacts and metadata"
	@echo "  package-artifacts           Refresh step build contexts and manifest metadata"
	@echo "  build-step-images           Build all step images"
	@echo "  install-local-pipeline      Install template locally and prepare step images"
	@echo "  build-pipeline-image        Build distributable pipeline image"
	@echo "  build                       Prepare local runnable pipeline"
	@echo "  dist                        Build distributable pipeline image"
	@echo "  test                        Run pipeline with NODE_FILE=$(NODE_FILE)"

clean:
	rm -rf /tmp/*
	rm -rf /var/lib/ar/runc/*
	rm -rf /var/lib/ar/tasks/*
	rm -rf /var/lib/ar/data/*
	# docker image prune -f
	df -h

build-step-images: package-artifacts
	sudo apt install skopeo -y
	@for dir in steps/*; do \
		if [ -d "$$dir" ]; then \
			$(MAKE) -C "$$dir" build; \
		fi; \
	done

install-local-pipeline: build-step-images
	mkdir -p /var/lib/ar/
	mkdir -p /var/lib/ar/pipelines/
	mkdir -p /var/lib/ar/images/
	mkdir -p /var/lib/ar/images/containerd-k8s-1_35_0-base
	cp "$(PIPELINE_TEMPLATE)" /var/lib/ar/pipelines/
	allrun pipeline list

build-pipeline-image: build-step-images
	allrun pipeline build -p "./$(PIPELINE_TEMPLATE)" -t "$(PIPELINE_IMAGE)" -i "./images.txt"

build: build-step-images install-local-pipeline

test: build
	allrun pipeline run -p $(PIPELINE_NAME) -n "$(NODE_FILE)"