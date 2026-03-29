SHELL := /bin/bash

GO ?= go
BUF ?= buf
BUF_VERSION ?= v1.66.1

.PHONY: all fmt vet lint buf-install buf-lint buf-generate test build

all: buf-lint buf-generate test build

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

lint: buf-lint vet

buf-install:
	$(GO) install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)

buf-lint:
	$(BUF) lint

buf-generate:
	$(BUF) generate

test:
	$(GO) test ./...

build:
	$(GO) build ./cmd/server
