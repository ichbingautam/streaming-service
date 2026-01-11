.PHONY: build run-api run-worker test lint clean docker-build docker-push

# Variables
APP_NAME=streaming-service
VERSION?=1.0.0
BUILD_DIR=./bin
DOCKER_REGISTRY?=your-registry

# Go settings
GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)
CGO_ENABLED?=0

# Build targets
build: build-api build-worker

build-api:
	@echo "Building API server..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="-s -w" -o $(BUILD_DIR)/api ./cmd/api

build-worker:
	@echo "Building worker..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="-s -w" -o $(BUILD_DIR)/worker ./cmd/worker

# Run targets
run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

# Test targets
test:
	go test -v -race -cover ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Lint
lint:
	golangci-lint run ./...

# Clean
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Dependencies
deps:
	go mod download
	go mod tidy

# Docker targets
docker-build:
	docker build -t $(APP_NAME)-api:$(VERSION) -f deployments/docker/Dockerfile.api .
	docker build -t $(APP_NAME)-worker:$(VERSION) -f deployments/docker/Dockerfile.worker .

docker-push:
	docker tag $(APP_NAME)-api:$(VERSION) $(DOCKER_REGISTRY)/$(APP_NAME)-api:$(VERSION)
	docker tag $(APP_NAME)-worker:$(VERSION) $(DOCKER_REGISTRY)/$(APP_NAME)-worker:$(VERSION)
	docker push $(DOCKER_REGISTRY)/$(APP_NAME)-api:$(VERSION)
	docker push $(DOCKER_REGISTRY)/$(APP_NAME)-worker:$(VERSION)

# Development
dev:
	air -c .air.toml

# Generate
generate:
	go generate ./...

# All in one
all: deps lint test build
