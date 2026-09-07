BINARY := prompiler
GO     ?= go

.PHONY: help build install clean fmt vet lint gen run \
        unit integration e2e test ci

.DEFAULT_GOAL := help

help: ## Print this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "\033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Compile the prompiler binary to bin/
	$(GO) build -o bin/$(BINARY) ./cmd/promptpiler

install: ## Install prompiler to GOPATH/bin
	$(GO) install ./cmd/promptpiler

fmt: ## Format all Go source files
	gofmt -w .

vet: ## Run go vet
	$(GO) vet ./...

lint: ## Check formatting and run vet
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed on:"; gofmt -l .; exit 1; }
	$(GO) vet ./...

gen: ## Regenerate the Wire wiring
	$(GO) generate ./...

unit: ## Run unit tests (skips the conformance integration test)
	$(GO) test ./... -short

integration: ## Run the examples fixture conformance harness
	$(GO) test ./internal/conformance/ -v

e2e: build ## Build and run the CLI smoke tests
	bash ./scripts/e2e.sh ./bin/$(BINARY)

test: ## Run the full suite (unit + integration)
	$(GO) test ./...

ci: lint unit integration e2e ## Run everything the CI pipeline runs

run: build ## Render a template (e.g. make run T=Interpolate ROOT=examples/feature/interpolation)
	./bin/$(BINARY) run -root $(ROOT) $(T)

clean: ## Remove build artifacts
	rm -rf bin/
