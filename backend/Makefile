APP_NAME := allrun
PKG_PATH := github.com/tangxusc/ar/backend/pkg/command

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
BUILT_BY:= $(shell whoami 2>/dev/null || echo "unknown")

.PHONY: build
build:
	go get github.com/99designs/gqlgen/codegen/config@v0.17.86
	go get github.com/99designs/gqlgen/internal/imports@v0.17.86
	go get github.com/99designs/gqlgen/codegen/config@v0.17.86
	go get github.com/99designs/gqlgen/api@v0.17.86
	go get github.com/99designs/gqlgen@v0.17.86
	go run github.com/99designs/gqlgen generate
	GOPROXY=https://proxy.golang.org,direct GOSUMDB=sum.golang.org go build -o $(APP_NAME) \
		-ldflags "\
			-X '$(PKG_PATH).Version=$(VERSION)' \
			-X '$(PKG_PATH).Commit=$(COMMIT)' \
			-X '$(PKG_PATH).Date=$(DATE)' \
			-X '$(PKG_PATH).BuiltBy=$(BUILT_BY)' \
		" \
		main.go


.PHONY: run
run:
	go run main.go server start