# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Project name and paths
PROJECT_NAME=fuel-monitor-api
BINARY_NAME=main
BINARY_UNIX=$(BINARY_NAME)_unix
MAIN_PATH=./cmd/api

# Docker parameters
DOCKER_IMAGE_NAME=fuel-monitor-api
DOCKER_TAG=latest

# Default target
.DEFAULT_GOAL := help

## Build the binary
build:
	$(GOBUILD) -o $(BINARY_NAME) -v $(MAIN_PATH)

## Build for Linux
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v $(MAIN_PATH)

## Clean build files
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)

## Run tests
test:
	$(GOTEST) -v ./...

## Run the application
run:
	$(GOBUILD) -o $(BINARY_NAME) -v $(MAIN_PATH) && ./$(BINARY_NAME)

## Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

## Build Docker image
docker-build:
	docker build -t $(DOCKER_IMAGE_NAME):$(DOCKER_TAG) .

## Run with Docker Compose
docker-up:
	docker-compose up --build

## Stop Docker Compose
docker-down:
	docker-compose down

## View Docker logs
docker-logs:
	docker-compose logs -f fuel-monitor-api

## Run database migration (placeholder for future)
migrate:
	@echo "Database migration functionality not implemented yet"

## Format code
fmt:
	go fmt ./...

## Lint code (requires golangci-lint)
lint:
	golangci-lint run

## Show this help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

.PHONY: build build-linux clean test run deps docker-build docker-up docker-down docker-logs migrate fmt lint help