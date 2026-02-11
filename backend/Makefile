APP_NAME := ar
PKG_PATH := github.com/tangxusc/ar/backend/pkg/command

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
BUILT_BY:= $(shell whoami 2>/dev/null || echo "unknown")

.PHONY: build
build:
	gqlgen generate
	go build -o $(APP_NAME) \
		-ldflags "\
			-X '$(PKG_PATH).Version=$(VERSION)' \
			-X '$(PKG_PATH).Commit=$(COMMIT)' \
			-X '$(PKG_PATH).Date=$(DATE)' \
			-X '$(PKG_PATH).BuiltBy=$(BUILT_BY)' \
		" \
		main.go

