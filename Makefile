# Makefile - build pb and run its tests. `make help` lists the targets.
#
# test is hermetic: no network and no .env. test-live sources .env and runs every
# PB_LIVE_* gate (tests named Live...) that needs no interactive login; a gate whose
# key is missing skips.

.DEFAULT_GOAL := help

ENV_FILE     ?= .env
WITH_ENV      = set -a; [ -f $(ENV_FILE) ] && . ./$(ENV_FILE); set +a;
LIVE_TIMEOUT ?= 30m

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z0-9_-]+:.*## / {printf "  \033[34m%-10s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build ./pb
	CGO_ENABLED=0 go build -trimpath -o pb .

.PHONY: test
test: ## Run the unit suite (no network, no .env)
	env -u EULERPOOL_API_KEY -u TAVILY_API_KEY \
		go test -count=1 ./...

.PHONY: test-live
test-live: ## Run the live gates with .env (Eulerpool, Tavily, EDGAR, the example screener)
	$(WITH_ENV) PB_LIVE_EULERPOOL=1 PB_LIVE_TAVILY=1 PB_LIVE_EDGAR=1 PB_LIVE_LIGHTYEAR=1 \
		go test -count=1 -timeout $(LIVE_TIMEOUT) -run Live ./...
