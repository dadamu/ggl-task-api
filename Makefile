APP            := server
BIN_DIR        := bin
LOCAL_BIN      := $(CURDIR)/$(BIN_DIR)/tools
GOLANGCI_VER   := v2.15.0
GOLANGCI_LINT  := $(LOCAL_BIN)/golangci-lint
SWAG_VER       := v1.16.6
SWAG           := $(LOCAL_BIN)/swag
COVER_OUT      := coverage.out

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_ -]+:.*##/ {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: swag
swag: $(SWAG) ## Generate swagger docs to swagger/
	$(SWAG) fmt
	$(SWAG) init -g cmd/server/main.go -o swagger --packageName swagger --parseInternal

.PHONY: build
build: swag ## Build binary to bin/
	go build -o $(BIN_DIR)/$(APP) ./cmd/server

.PHONY: run
run: swag ## Run server
	go run ./cmd/server

.PHONY: test
test: ## Run tests with race detector
	go test -race -count=1 ./...

.PHONY: cover coverage
cover coverage: ## Coverage report for internal/ (excludes cmd/ and generated swagger/)
	go test -race -count=1 -covermode=atomic -coverpkg=./internal/... -coverprofile=$(COVER_OUT) ./internal/...
	go tool cover -func=$(COVER_OUT)

.PHONY: fmt
fmt: $(GOLANGCI_LINT) ## Format code
	$(GOLANGCI_LINT) fmt ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Run golangci-lint
	$(GOLANGCI_LINT) run ./...

.PHONY: lint-fix
lint-fix: $(GOLANGCI_LINT) ## Run golangci-lint with auto-fix
	$(GOLANGCI_LINT) run --fix ./...

.PHONY: tidy
tidy: ## Tidy go.mod
	go mod tidy

.PHONY: check
check: swag tidy fmt vet lint test ## Run all checks (CI)

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) $(COVER_OUT)

$(GOLANGCI_LINT):
	GOBIN=$(LOCAL_BIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VER)

$(SWAG):
	GOBIN=$(LOCAL_BIN) go install github.com/swaggo/swag/cmd/swag@$(SWAG_VER)
