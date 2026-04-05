VERSION ?= 0.1.0
BINARY_NAME ?= mcp-auth-gateway
IMG ?= ghcr.io/nicknikolakakis/$(BINARY_NAME):v$(VERSION)

GOBIN = $(shell pwd)/bin
GOLANGCI_LINT_VERSION ?= v2.1.6

.PHONY: all
all: build

## Build

.PHONY: build
build:
	go build -trimpath -ldflags="-s -w" -o bin/$(BINARY_NAME) cmd/gateway/main.go

.PHONY: run
run: build
	./bin/$(BINARY_NAME) --config config.yaml

## Test

.PHONY: test
test:
	go test -race -coverprofile=cover.out ./...

.PHONY: test-verbose
test-verbose:
	go test -race -v -coverprofile=cover.out ./...

## Lint

.PHONY: lint
lint: golangci-lint
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint
	$(GOLANGCI_LINT) run --fix

## Docker

.PHONY: docker-build
docker-build:
	docker build -t $(IMG) .

.PHONY: docker-push
docker-push:
	docker push $(IMG)

## Format

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

## Clean

.PHONY: clean
clean:
	rm -rf bin/ cover.out

## Tool dependencies

GOLANGCI_LINT = $(GOBIN)/golangci-lint
.PHONY: golangci-lint
golangci-lint:
	@test -s $(GOLANGCI_LINT) || \
	GOBIN=$(GOBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
