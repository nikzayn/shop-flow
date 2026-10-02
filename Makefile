.DEFAULT_GOAL := help
.PHONY: help lint test

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

lint: ## Run all pre-commit checks
	pre-commit run --all-files

test: ## Run tests
	@echo "No tests yet"
