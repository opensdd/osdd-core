ROOT := $(shell git rev-parse --show-toplevel)
export GOBIN := $(ROOT)/bin

$(GOBIN):
	@mkdir -p $(GOBIN)
	@echo "$(GOBIN)" created

$(GOBIN)/golangci-lint: $(GOBIN)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(GOBIN) v2.5.0
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