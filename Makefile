ROOT := $(shell git rev-parse --show-toplevel)
export GOBIN := $(ROOT)/bin

$(GOBIN):
	@mkdir -p $GOBIN
	@echo "$GOBIN" created

$(GOBIN)/golangci-lint: $(GOBIN)
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint
	@echo "golangci-lint install"

lint: $(GOBIN)/golangci-lint
	$(GOBIN)/golangci-lint run

test:
	go test ./...

test-integration:
	go test ./... -tags=integration

test-all: test-integration

setup: $(GOBIN)
	@go mod tidy

.PHONY: lint test test-unit test-integration test-all test-verbose test-coverage test-coverage-integration setup