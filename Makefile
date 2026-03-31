GO ?= go
GOLANGCI_LINT ?= golangci-lint

.PHONY: lint test verify

lint:
	$(GOLANGCI_LINT) run -c .golangci.yml ./...

test:
	$(GO) test ./...

verify: test lint
