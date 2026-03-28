SHELL := /bin/bash

PIPELINE_NAME := uninstall-containerd-k8s
PIPELINE_TEMPLATE := uninstall-containerd-k8s.template.json
PIPELINE_IMAGE := pipeline-$(PIPELINE_NAME):latest
PIPELINE_ROOT := $(CURDIR)
NODE_FILE ?= testdata/node.json
ARGS_FILE ?= testdata/args.json

.PHONY: help prepare-dirs build-step-images install-local-pipeline build-pipeline-image build dist test

help:
	@echo "Targets:"
	@echo "  build-step-images           Build all step images"
	@echo "  install-local-pipeline      Install template locally and prepare step images"
	@echo "  build-pipeline-image        Build distributable pipeline image"
	@echo "  build                       Prepare local runnable pipeline"
	@echo "  dist                        Build distributable pipeline image"
	@echo "  test                        Run pipeline with NODE_FILE=$(NODE_FILE) ARGS_FILE=$(ARGS_FILE)"

build-step-images:
	mkdir -p /var/lib/ar
	mkdir -p /var/lib/ar/pipelines/
	mkdir -p /var/lib/ar/images/
	@for dir in steps/*; do \
		if [ -d "$$dir" ]; then \
			$(MAKE) -C "$$dir" build; \
		fi; \
	done

install-local-pipeline: build-step-images
	cp "$(PIPELINE_TEMPLATE)" /var/lib/ar/pipelines/
	allrun pipeline list

build-pipeline-image: build-step-images
	allrun pipeline build -p "./$(PIPELINE_TEMPLATE)" -t "$(PIPELINE_IMAGE)" -i "./images.txt"

build: build-step-images install-local-pipeline

dist: build-pipeline-image

test: build
	allrun pipeline run -p $(PIPELINE_NAME) -n "$(NODE_FILE)" -a "$(ARGS_FILE)"
