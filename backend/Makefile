BACKEND_DIR := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
GO_MODULES := services/identity services/organization services/access services/audit cmd/bootstrap-superadmin
COMPOSE = cd "$(BACKEND_DIR)" && docker compose

.PHONY: format format-check check-go-modules test vet build security quality db-config-check db-check db-up db-down db-status db-logs

format:
	@find "$(BACKEND_DIR)/services" "$(BACKEND_DIR)/cmd" -type f -name '*.go' -print0 | xargs -0 -r gofmt -w

format-check:
	@set -eu; \
	files="$$(find "$(BACKEND_DIR)/services" "$(BACKEND_DIR)/cmd" -type f -name '*.go' -print0 | xargs -0 -r gofmt -l)"; \
	if [ -n "$$files" ]; then \
		printf '%s\n' "Go files need formatting:" "$$files" >&2; \
		exit 1; \
	fi

check-go-modules:
	@set -eu; \
	for module in $(GO_MODULES); do \
		if [ ! -s "$(BACKEND_DIR)/$$module/go.mod" ]; then \
			printf 'Missing or empty Go module manifest: %s/go.mod\n' "$$module" >&2; \
			exit 1; \
		fi; \
	done

test vet build: check-go-modules

test:
	@set -eu; \
	for module in $(GO_MODULES); do \
		printf '==> go test ./... (%s)\n' "$$module"; \
		(cd "$(BACKEND_DIR)/$$module" && go test ./...); \
	done

vet:
	@set -eu; \
	for module in $(GO_MODULES); do \
		printf '==> go vet ./... (%s)\n' "$$module"; \
		(cd "$(BACKEND_DIR)/$$module" && go vet ./...); \
	done

build:
	@set -eu; \
	for module in $(GO_MODULES); do \
		printf '==> go build ./... (%s)\n' "$$module"; \
		(cd "$(BACKEND_DIR)/$$module" && go build ./...); \
	done

security: check-go-modules
	@command -v govulncheck >/dev/null 2>&1 || { \
		printf '%s\n' 'govulncheck is required; install the approved golang.org/x/vuln scanner version and retry.' >&2; \
		exit 1; \
	}
	@set -eu; \
	for module in $(GO_MODULES); do \
		printf '==> govulncheck ./... (%s)\n' "$$module"; \
		(cd "$(BACKEND_DIR)/$$module" && govulncheck ./...); \
	done

quality: format-check test vet build security db-config-check

db-config-check:
	@sh "$(BACKEND_DIR)/scripts/check-db.sh" config

db-check:
	@sh "$(BACKEND_DIR)/scripts/check-db.sh" ready

db-up:
	$(COMPOSE) up --detach postgres

db-down:
	$(COMPOSE) down

db-status:
	$(COMPOSE) ps

db-logs:
	$(COMPOSE) logs --follow postgres
