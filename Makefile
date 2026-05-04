PLUGIN_NAME := kubescape
PLUGIN_DIR  := $(shell pwd)
BIN         := $(PLUGIN_DIR)/bin/helm-kubescape

.PHONY: help build test lint vet install uninstall reinstall clean

help: ## Show this help.
	@awk 'BEGIN{FS=":.*##";printf "\nTargets:\n"} /^[a-zA-Z_-]+:.*##/{printf "  \033[36m%-14s\033[0m %s\n",$$1,$$2}' $(MAKEFILE_LIST)

build: ## Build the helm-kubescape binary into bin/.
	go build -trimpath -ldflags='-s -w' -o $(BIN) ./cmd/helm-kubescape

test: ## Run Go unit tests.
	go test ./...

vet: ## Run go vet.
	go vet ./...

lint: vet ## Run all linters (currently just go vet).

install: ## Install this checkout as a local Helm plugin (helm plugin install <path>).
	helm plugin install $(PLUGIN_DIR)

uninstall: ## Remove the locally installed plugin.
	helm plugin uninstall $(PLUGIN_NAME) || true

reinstall: uninstall install ## Uninstall + install (use after editing source).

clean: ## Remove built artifacts.
	rm -rf $(PLUGIN_DIR)/bin
