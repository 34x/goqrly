.PHONY: build test clean install run fmt lint help

# Binary name
BINARY := goqrly

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOFMT := $(GOCMD) fmt
GOVET := $(GOCMD) vet

# Build flags
LDFLAGS := -s -w

# Default target
all: build

## build: Build the binary
build:
	$(GOBUILD) -ldflags="$(LDFLAGS)" -o $(BINARY) .

## test: Run all tests (unit + integration)
test: build
	./test.sh

## unit-test: Run unit tests only
unit-test:
	$(GOTEST) -v ./...

## integration-test: Run integration tests only
integration-test: build
	$(GOTEST) -v -run Integration ./...

## clean: Remove build artifacts and test files
clean:
	$(GOCLEAN)
	-rm -f $(BINARY)
	-rm -rf tmp_test_data
	-rm -f /tmp/goqrly*.txt /tmp/goqrly.pid

## fmt: Format source code
fmt:
	$(GOFMT) ./...

## vet: Run go vet
vet:
	$(GOVET) ./...

## lint: Run all linters (fmt + vet)
lint: fmt vet

## run: Build and run locally (default port 8080)
run: build
	./$(BINARY)

## run-persist: Build and run with persistent storage
run-persist: build
	-rm -rf ./data
	./$(BINARY) --data-dir ./data

## install: Install as systemd service (requires root)
install: build
	./$(BINARY) install

## help: Show this help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':'