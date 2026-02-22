BINARY_NAME := sidequest
BUILD_DIR := bin
MAIN_PATH := ./cmd/sidequest
MODULE := github.com/MisterGrinvalds/sidequest

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

GOFLAGS := -ldflags="-s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)"

# --- Build ---

.PHONY: build
build: ## Build the sidequest binary
	go build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

.PHONY: run
run: build ## Build and run sidequest
	$(BUILD_DIR)/$(BINARY_NAME)

.PHONY: install
install: ## Install sidequest to $GOPATH/bin
	go install $(GOFLAGS) $(MAIN_PATH)

# --- Test ---

.PHONY: test
test: ## Run all tests
	go test ./... -v

.PHONY: test-cover
test-cover: ## Run tests with coverage report
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: test-race
test-race: ## Run tests with race detector
	go test ./... -race -v

# --- Lint ---

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: fmt
fmt: ## Format Go code
	gofmt -s -w .

.PHONY: vet
vet: ## Run go vet
	go vet ./...

# --- Code Generation ---

.PHONY: proto
proto: ## Generate protobuf/gRPC code
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/sidequest/v1/*.proto

.PHONY: graphql
graphql: ## Generate gqlgen code
	go run github.com/99designs/gqlgen generate

.PHONY: generate
generate: proto graphql ## Run all code generation

# --- Docker ---

IMAGE_NAME := sidequest
IMAGE_TAG  ?= $(VERSION)
IMAGE_REF   = $(IMAGE_NAME):$(IMAGE_TAG)

.PHONY: docker-build
docker-build: ## Build container image for local arch
	docker build -t $(IMAGE_REF) .

# --- Docker (via lazyoci, multi-arch) ---

.PHONY: docker-build-all
docker-build-all: ## Build multi-arch image via lazyoci (no push)
	lazyoci build --no-push --tag $(IMAGE_TAG)

.PHONY: docker-push
docker-push: ## Build and push multi-arch image via lazyoci
	LAZYOCI_REGISTRY=registry.digitalocean.com/greenforests lazyoci build --tag $(IMAGE_TAG)

.PHONY: docker-dry-run
docker-dry-run: ## Preview multi-arch build (dry run)
	LAZYOCI_REGISTRY=registry.digitalocean.com/greenforests lazyoci build --dry-run --tag $(IMAGE_TAG)

# --- KIND ---

KIND_CLUSTER ?= greenforests-local

.PHONY: kind-load
kind-load: docker-build ## Build image and load into KIND cluster
	kind load docker-image $(IMAGE_REF) --name $(KIND_CLUSTER)

.PHONY: kind-deploy
kind-deploy: kind-load ## Build, load into KIND, and deploy
	$(BUILD_DIR)/$(BINARY_NAME) deploy k8s --image $(IMAGE_REF)

.PHONY: kind-undeploy
kind-undeploy: ## Remove sidequest from KIND cluster
	$(BUILD_DIR)/$(BINARY_NAME) undeploy k8s

# --- Clean ---

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR) coverage.out coverage.html

# --- Help ---

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
