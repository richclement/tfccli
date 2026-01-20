SHELL := /bin/bash

# `make` should build the binary by default.
.DEFAULT_GOAL := build

.PHONY: build tfc tfc-help help fmt fmt-check lint test ci tools clean install all

BIN_DIR := $(CURDIR)/bin
BIN := $(BIN_DIR)/tfc
CMD := ./cmd/tfc

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo "")
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

TOOLS_DIR := $(CURDIR)/.tools
GOFUMPT := $(TOOLS_DIR)/gofumpt
GOIMPORTS := $(TOOLS_DIR)/goimports
GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint

build:
	@mkdir -p $(BIN_DIR)
	@go build -ldflags "$(LDFLAGS)" -o $(BIN) $(CMD)

tfc: build
	@if [ -z "$(ARGS)" ]; then \
		$(BIN) --help; \
	else \
		$(BIN) $(ARGS); \
	fi

tfc-help: build
	@$(BIN) --help

help: tfc-help

tools:
	@mkdir -p $(TOOLS_DIR)
	@GOBIN=$(TOOLS_DIR) go install mvdan.cc/gofumpt@v0.7.0
	@GOBIN=$(TOOLS_DIR) go install golang.org/x/tools/cmd/goimports@v0.28.0
	@GOBIN=$(TOOLS_DIR) go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.62.2

fmt: tools
	@$(GOIMPORTS) -local github.com/richclement/tfccli -w .
	@$(GOFUMPT) -w .

fmt-check: tools
	@$(GOIMPORTS) -local github.com/richclement/tfccli -w .
	@$(GOFUMPT) -w .
	@git diff --exit-code -- '*.go' go.mod go.sum

lint: tools
	@$(GOLANGCI_LINT) run

test:
	@go test ./...

ci: fmt-check lint test

clean:
	rm -f tfc
	rm -rf bin/
	rm -rf dist/
	rm -rf .tools/

install:
	go install -ldflags "$(LDFLAGS)" $(CMD)

all: fmt lint test build