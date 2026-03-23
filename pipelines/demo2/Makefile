SHELL := /bin/bash

PIPELINE_NAME := demo2
PIPELINE_TEMPLATE := demo2.template.json
PIPELINE_IMAGE := demo2:latest
NODE_FILE ?= testdata/node.json

clean:
	rm -rf /tmp/*
	rm -rf /var/lib/ar/runc/*
	rm -rf /var/lib/ar/tasks/*
	rm -rf /var/lib/ar/data/*
	df -h

install-local-pipeline: 
	mkdir -p /var/lib/ar/
	mkdir -p /var/lib/ar/pipelines/
	mkdir -p /var/lib/ar/images/
	mkdir -p /var/lib/ar/images/containerd-k8s-1_35_0-base
	cp "$(PIPELINE_TEMPLATE)" /var/lib/ar/pipelines/
	allrun pipeline list

build-pipeline-image: 
	allrun pipeline build -p "./$(PIPELINE_TEMPLATE)" -t "$(PIPELINE_IMAGE)" -i "./images.txt"

test: build-pipeline-image install-local-pipeline
	allrun pipeline run -p $(PIPELINE_NAME) -n "$(NODE_FILE)"