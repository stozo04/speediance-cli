# speediance-cli — common developer tasks.
# Run `make help` for the list. Recipes use tabs (Make requirement).

BINARY      := speediance-cli
PKG         := ./...
MAIN        := ./cmd/speediance-cli
LOCAL_PREFIX := github.com/stozo04/speediance-cli

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the binary into ./bin
	go build -o bin/$(BINARY) $(MAIN)

.PHONY: install
install: ## go install the binary into GOBIN
	go install $(MAIN)

.PHONY: test
test: ## Run tests
	go test $(PKG)

.PHONY: test-race
test-race: ## Run tests with the race detector
	go test -race $(PKG)

.PHONY: cover
cover: ## Run tests with coverage and open the report
	go test -coverprofile=coverage.out $(PKG)
	go tool cover -html=coverage.out

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run $(PKG)

.PHONY: fmt
fmt: ## Format with gofumpt + goimports
	gofumpt -w .
	goimports -w -local $(LOCAL_PREFIX) .

.PHONY: tidy
tidy: ## Tidy go.mod/go.sum
	go mod tidy

.PHONY: vet
vet: ## Run go vet
	go vet $(PKG)

.PHONY: snapshot
snapshot: ## Cross-compile a local snapshot with GoReleaser (no publish)
	goreleaser build --snapshot --clean

.PHONY: check
check: tidy fmt vet lint test-race ## Run the full pre-commit gauntlet

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin dist coverage.out $(BINARY) $(BINARY).exe
