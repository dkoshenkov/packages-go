GO ?= go
GOLANGCI_LINT_VERSION ?= v2.15.0
GOLANGCI_LINT ?= $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: lint test verify

lint:
	$(GOLANGCI_LINT) run -c .golangci.yml ./...

test:
	$(GO) test ./...

verify: test lint
