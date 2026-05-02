PLUGIN_NAME := kubescape
PLUGIN_DIR  := $(shell pwd)

.PHONY: help install uninstall reinstall lint test test-shellcheck smoke

help: ## Show this help.
	@awk 'BEGIN{FS=":.*##";printf "\nTargets:\n"} /^[a-zA-Z_-]+:.*##/{printf "  \033[36m%-14s\033[0m %s\n",$$1,$$2}' $(MAKEFILE_LIST)

install: ## Install this checkout as a local Helm plugin (helm plugin install <path>).
	helm plugin install $(PLUGIN_DIR)

uninstall: ## Remove the locally installed plugin.
	helm plugin uninstall $(PLUGIN_NAME) || true

reinstall: uninstall install ## Uninstall + install (use after editing scripts).

lint: test-shellcheck ## Run all linters.

test-shellcheck: ## Run shellcheck against the plugin scripts.
	shellcheck scripts/helm-kubescape.sh install-binary.sh

test: ## Run plugin self-tests (requires a local kubescape build).
	bash test/smoke.sh
