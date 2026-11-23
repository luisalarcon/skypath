STATICCHECK_VERSION := 2026.2.2
MODULES             := backend frontend
BACKEND_PORT        ?= 8080
FRONTEND_PORT       ?= 3000

.DEFAULT_GOAL := help
.PHONY: help test vet fmt fmt-check lint build up down logs smoke

help: ## List available targets
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  %-10s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

test: ## Run unit tests with the race detector in every module
	@for m in $(MODULES); do echo "==> $$m"; (cd $$m && go test -race ./...) || exit 1; done

vet: ## Run go vet in every module
	@for m in $(MODULES); do echo "==> $$m"; (cd $$m && go vet ./...) || exit 1; done

fmt: ## Format all Go sources in place
	@for m in $(MODULES); do (cd $$m && gofmt -w .) || exit 1; done

fmt-check: ## Fail if any Go source is not gofmt-formatted
	@for m in $(MODULES); do \
		out=$$(cd $$m && gofmt -l .); \
		if [ -n "$$out" ]; then echo "$$m: unformatted files:"; echo "$$out"; exit 1; fi; \
	done

lint: ## Run staticcheck in every module
	@for m in $(MODULES); do echo "==> $$m"; (cd $$m && go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) ./...) || exit 1; done

build: ## Build both container images
	docker compose build

up: ## Build and start the stack in the background
	docker compose up -d --build --wait

down: ## Stop and remove the stack
	docker compose down

logs: ## Follow logs from the running stack
	docker compose logs -f

smoke: ## Check the running stack responds and can search
	curl -fsS http://localhost:$(BACKEND_PORT)/healthz
	@echo
	curl -fsS http://localhost:$(BACKEND_PORT)/readyz
	@echo
	curl -fsS http://localhost:$(FRONTEND_PORT)/healthz
	@echo
	curl -fsS -o /dev/null -w "frontend / -> %{http_code}\n" http://localhost:$(FRONTEND_PORT)/
	curl -fsS "http://localhost:$(BACKEND_PORT)/api/v1/search?origin=JFK&destination=LAX&date=2024-03-15" | grep -q '"count":27' && echo "search JFK-LAX -> 27 itineraries"
