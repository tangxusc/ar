SHELL := /bin/bash

PIPELINE_NAME := demo4
PIPELINE_TEMPLATE := demo4.template.json
PIPELINE_IMAGE := demo4:latest
NODE_FILE ?= testdata/node.json
ARGS_FILE ?= testdata/args.json

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
	cp "$(PIPELINE_TEMPLATE)" /var/lib/ar/pipelines/
	allrun pipeline list

build-pipeline-image:
	allrun pipeline build -p "./$(PIPELINE_TEMPLATE)" -t "$(PIPELINE_IMAGE)" -i "./images.txt"

test: build-pipeline-image install-local-pipeline
	allrun pipeline run -p $(PIPELINE_NAME) -n "$(NODE_FILE)" -a "$(ARGS_FILE)"
