.PHONY: help fmt fmt-check vet test test-race api-run db-migrate db-status test-integration-mysql lesson28-mysql-acceptance lesson30-mysql-acceptance doc-check web-install web-test web-typecheck web-build web-verify compose-secrets compose-config compose-build compose-up compose-down compose-reset compose-ps compose-logs compose-migrate compose-grants compose-status compose-identity-provision compose-identity-maintenance compose-smoke compose-lottery-api-acceptance compose-load-health compose-load-ready compose-verify compose-m0 docs-sync docs-sync-watch verify

COMPOSE_FILE ?= deploy/compose/compose.yaml
COMPOSE_PROJECT ?= growthos
COMPOSE = docker compose --project-name $(COMPOSE_PROJECT) --file $(COMPOSE_FILE)
GROWTHOS_COMPOSE_WEB_PORT ?= 8088
HEALTHLOAD_RATE ?= 100
HEALTHLOAD_DURATION ?= 5m
HEALTHLOAD_WORKERS ?= 32
HEALTHLOAD_TIMEOUT ?= 2s
HEALTHLOAD_MAX_P99 ?= 100ms
READYLOAD_RATE ?= 20
READYLOAD_DURATION ?= 30s

# Command-line variables are recursively expanded by GNU Make when exported.
# A dollar could therefore turn even an invalid caller value into a Make
# function before shell quoting applies. The one-shot Identity targets reject
# dollars from raw, unexpanded Make values at parse time (their direct wrappers
# remain available for an unusual pathname containing one). Remaining bytes
# travel only through quoted, target-scoped shell environment variables, so
# backticks and other invalid characters are not evaluated a second time.
ifneq ($(filter compose-identity-provision compose-identity-maintenance,$(MAKECMDGOALS)),)
ifneq (,$(findstring $$,$(value COMPOSE_PROJECT)))
$(error COMPOSE_PROJECT must not contain a dollar sign)
endif
ifneq (,$(findstring $$,$(value COMPOSE_FILE)))
$(error COMPOSE_FILE must not contain a dollar sign; invoke the operation wrapper directly for that pathname)
endif
ifneq (,$(findstring $$,$(value GROWTHOS_COMPOSE_WEB_PORT)))
$(error GROWTHOS_COMPOSE_WEB_PORT must not contain a dollar sign)
endif
ifneq (,$(findstring $$,$(value GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID)))
$(error GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID must not contain a dollar sign)
endif
endif

ifneq ($(filter compose-identity-provision,$(MAKECMDGOALS)),)
ifneq (,$(findstring $$,$(value IDENTITY_ACCOUNT_ID)))
$(error IDENTITY_ACCOUNT_ID must not contain a dollar sign)
endif
ifneq (,$(findstring $$,$(value IDENTITY_LOGIN_NAME)))
$(error IDENTITY_LOGIN_NAME must not contain a dollar sign)
endif
ifneq (,$(findstring $$,$(value IDENTITY_PRINCIPAL_ID)))
$(error IDENTITY_PRINCIPAL_ID must not contain a dollar sign)
endif
ifneq (,$(findstring $$,$(value IDENTITY_PASSWORD_FILE)))
$(error IDENTITY_PASSWORD_FILE must not contain a dollar sign; call scripts/compose-identity-provision.sh directly for that pathname)
endif
endif

help:
	@printf '%s\n' \
		'Available targets:' \
		'  make fmt        Format Go code' \
		'  make fmt-check  Fail when Go code is not formatted' \
		'  make vet        Run Go static analysis' \
		'  make test       Run Go tests' \
		'  make test-race  Run Go tests with the race detector' \
		'  make api-run    Run the GrowthOS API locally' \
		'  make db-migrate Apply all pending forward MySQL migrations' \
		'  make db-status  Inspect the current MySQL migration status' \
		'  make test-integration-mysql  Verify MySQL migrations, permissions, and Lottery repository' \
		'  make lesson28-mysql-acceptance  Run the disposable MySQL 8.4.11 Lesson 28 gate' \
		'  make lesson30-mysql-acceptance  Run the confirmed disposable MySQL 8.4.11 Lesson 30 gate' \
		'  make doc-check  Check documentation integrity and course evidence' \
		'  make web-test   Run frontend unit and component tests' \
		'  make web-verify Run frontend tests, typecheck, and production build' \
		'  make compose-up Start the isolated local Compose stack and wait for health' \
		'  make compose-migrate Apply migrations and reconcile application grants' \
		'  make compose-grants Reconcile the exact application table-grant allowlist' \
		'  make compose-status Inspect migration state with the freshly built image' \
		'  make compose-identity-provision  Create one local Identity account from a private password file' \
		'  make compose-identity-maintenance  Run one bounded local Identity session cleanup' \
		'  make compose-down Stop the Compose stack while retaining named volumes' \
		'  make compose-ps  Show Compose services and health' \
		'  make compose-smoke Verify normal stack state, HTTP contracts, and port isolation' \
		'  make compose-lottery-api-acceptance  Run the disposable current Lottery/cache acceptance' \
		'  make compose-m0  Run the explicit M0 smoke and load acceptance gate' \
		'  make verify     Run all local quality gates'

fmt:
	gofmt -w $$(find . -type f -name '*.go' -not -path './vendor/*')

fmt-check:
	@test -z "$$(gofmt -l $$(find . -type f -name '*.go' -not -path './vendor/*'))" || \
		(printf '%s\n' 'Go files require formatting. Run make fmt.' && exit 1)

vet:
	go vet ./...

test:
	go test ./...

test-race:
	go test -race ./...

api-run:
	go run ./cmd/growth-api

db-migrate:
	go run ./cmd/growth-migrate up

db-status:
	go run ./cmd/growth-migrate status

test-integration-mysql:
	@test "$${GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES:-}" = 'lesson-19-isolated-schema' || (printf '%s\n' 'set GROWTHOS_TEST_MYSQL_ALLOW_SCHEMA_CHANGES=lesson-19-isolated-schema for a dedicated disposable test schema' && exit 1)
	@test "$${GROWTHOS_TEST_MYSQL_ALLOW_REPOSITORY_WRITES:-}" = 'lesson-19-isolated-repository' || (printf '%s\n' 'set GROWTHOS_TEST_MYSQL_ALLOW_REPOSITORY_WRITES=lesson-19-isolated-repository for isolated repository writes' && exit 1)
	@test "$${GROWTHOS_TEST_MYSQL_ALLOW_RULE_GRAPH_WRITES:-}" = 'lesson-28-isolated-rule-graph' || (printf '%s\n' 'set GROWTHOS_TEST_MYSQL_ALLOW_RULE_GRAPH_WRITES=lesson-28-isolated-rule-graph for isolated rule-graph writes' && exit 1)
	@test -n "$${GROWTHOS_TEST_MYSQL_API_ADDRESS:-}" || (printf '%s\n' 'missing required variable: GROWTHOS_TEST_MYSQL_API_ADDRESS' && exit 1)
	@test -n "$${GROWTHOS_TEST_MYSQL_API_DATABASE:-}" || (printf '%s\n' 'missing required variable: GROWTHOS_TEST_MYSQL_API_DATABASE' && exit 1)
	@test -n "$${GROWTHOS_TEST_MYSQL_API_USER:-}" || (printf '%s\n' 'missing required variable: GROWTHOS_TEST_MYSQL_API_USER' && exit 1)
	@test -n "$${GROWTHOS_TEST_MYSQL_API_PASSWORD:-}" || (printf '%s\n' 'missing required variable: GROWTHOS_TEST_MYSQL_API_PASSWORD' && exit 1)
	@test -n "$${GROWTHOS_TEST_MYSQL_MIGRATION_ADDRESS:-}" || (printf '%s\n' 'missing required variable: GROWTHOS_TEST_MYSQL_MIGRATION_ADDRESS' && exit 1)
	@test -n "$${GROWTHOS_TEST_MYSQL_MIGRATION_DATABASE:-}" || (printf '%s\n' 'missing required variable: GROWTHOS_TEST_MYSQL_MIGRATION_DATABASE' && exit 1)
	@test -n "$${GROWTHOS_TEST_MYSQL_MIGRATION_USER:-}" || (printf '%s\n' 'missing required variable: GROWTHOS_TEST_MYSQL_MIGRATION_USER' && exit 1)
	@test -n "$${GROWTHOS_TEST_MYSQL_MIGRATION_PASSWORD:-}" || (printf '%s\n' 'missing required variable: GROWTHOS_TEST_MYSQL_MIGRATION_PASSWORD' && exit 1)
	@test -n "$${GROWTHOS_TEST_MYSQL_RULE_GRAPH_ADDRESS:-}" || (printf '%s\n' 'missing required variable: GROWTHOS_TEST_MYSQL_RULE_GRAPH_ADDRESS' && exit 1)
	@test -n "$${GROWTHOS_TEST_MYSQL_RULE_GRAPH_DATABASE:-}" || (printf '%s\n' 'missing required variable: GROWTHOS_TEST_MYSQL_RULE_GRAPH_DATABASE' && exit 1)
	@test -n "$${GROWTHOS_TEST_MYSQL_RULE_GRAPH_USER:-}" || (printf '%s\n' 'missing required variable: GROWTHOS_TEST_MYSQL_RULE_GRAPH_USER' && exit 1)
	@test -n "$${GROWTHOS_TEST_MYSQL_RULE_GRAPH_PASSWORD:-}" || (printf '%s\n' 'missing required variable: GROWTHOS_TEST_MYSQL_RULE_GRAPH_PASSWORD' && exit 1)
	go test -v -count=1 -p=1 -run 'Integration$$' \
		./internal/infrastructure/mysql \
		./internal/infrastructure/migration \
		./migrations \
		./internal/lottery/adapter/mysqlrepo

lesson28-mysql-acceptance:
	./scripts/lesson28-mysql-acceptance.sh

lesson30-mysql-acceptance:
	@test "$${GROWTHOS_LESSON30_MYSQL_ACCEPTANCE:-}" = 'run-disposable-mysql-8.4.11' || (printf '%s\n' 'set GROWTHOS_LESSON30_MYSQL_ACCEPTANCE=run-disposable-mysql-8.4.11 to authorize the one-time tmpfs container' && exit 1)
	./scripts/lesson30-mysql-acceptance.sh

doc-check:
	go run ./cmd/doccheck

web-install:
	cd web && pnpm install --frozen-lockfile

web-test:
	cd web && pnpm run test

web-typecheck:
	cd web && pnpm run typecheck

web-build:
	cd web && pnpm run build

web-verify: web-test web-typecheck web-build

compose-secrets:
	GROWTHOS_COMPOSE_PROJECT="$(COMPOSE_PROJECT)" \
	GROWTHOS_COMPOSE_WEB_PORT="$(GROWTHOS_COMPOSE_WEB_PORT)" \
	GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID="$${GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID:-local-v1}" \
	./scripts/generate-compose-secrets.sh

compose-config: compose-secrets
	$(COMPOSE) config --quiet

compose-build: compose-config
	$(COMPOSE) build

compose-up: compose-config
	$(COMPOSE) up --detach --build --wait --wait-timeout 180

compose-down:
	$(COMPOSE) down --remove-orphans

compose-reset:
	@test "$(CONFIRM)" = 'reset-growthos-data' || (printf '%s\n' 'refusing to delete the Compose data and socket volumes; retry with CONFIRM=reset-growthos-data' >&2 && exit 1)
	$(COMPOSE) down --volumes --remove-orphans

compose-ps:
	$(COMPOSE) ps

compose-logs:
	$(COMPOSE) logs --tail=200

compose-migrate: compose-secrets
	$(COMPOSE) run --rm --build migrate up
	$(COMPOSE) run --rm --no-deps mysql-grants

compose-grants: compose-secrets
	$(COMPOSE) run --rm --no-deps mysql-grants

compose-status: compose-secrets
	$(COMPOSE) run --rm --build migrate status

compose-identity-provision: export IDENTITY_ACCOUNT_ID := $(value IDENTITY_ACCOUNT_ID)
compose-identity-provision: export IDENTITY_LOGIN_NAME := $(value IDENTITY_LOGIN_NAME)
compose-identity-provision: export IDENTITY_PRINCIPAL_ID := $(value IDENTITY_PRINCIPAL_ID)
compose-identity-provision: export IDENTITY_PASSWORD_FILE := $(value IDENTITY_PASSWORD_FILE)
compose-identity-provision: export COMPOSE_PROJECT := $(value COMPOSE_PROJECT)
compose-identity-provision: export COMPOSE_FILE := $(value COMPOSE_FILE)
compose-identity-provision: export GROWTHOS_COMPOSE_WEB_PORT := $(value GROWTHOS_COMPOSE_WEB_PORT)
compose-identity-provision: export GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID := $(value GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID)
compose-identity-provision:
	@test -n "$$IDENTITY_ACCOUNT_ID" || (printf '%s\n' 'set IDENTITY_ACCOUNT_ID' >&2 && exit 2)
	@test -n "$$IDENTITY_LOGIN_NAME" || (printf '%s\n' 'set IDENTITY_LOGIN_NAME' >&2 && exit 2)
	@test -n "$$IDENTITY_PRINCIPAL_ID" || (printf '%s\n' 'set IDENTITY_PRINCIPAL_ID' >&2 && exit 2)
	@test -n "$$IDENTITY_PASSWORD_FILE" || (printf '%s\n' 'set IDENTITY_PASSWORD_FILE to a caller-owned 0600 regular file' >&2 && exit 2)
	GROWTHOS_COMPOSE_PROJECT="$$COMPOSE_PROJECT" \
	GROWTHOS_COMPOSE_FILE="$$COMPOSE_FILE" \
	GROWTHOS_COMPOSE_WEB_PORT="$$GROWTHOS_COMPOSE_WEB_PORT" \
	GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID="$${GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID:-local-v1}" \
	./scripts/compose-identity-provision.sh \
		--account-id "$$IDENTITY_ACCOUNT_ID" \
		--login-name "$$IDENTITY_LOGIN_NAME" \
		--principal-id "$$IDENTITY_PRINCIPAL_ID" \
		--password-file "$$IDENTITY_PASSWORD_FILE"

compose-identity-maintenance: export COMPOSE_PROJECT := $(value COMPOSE_PROJECT)
compose-identity-maintenance: export COMPOSE_FILE := $(value COMPOSE_FILE)
compose-identity-maintenance: export GROWTHOS_COMPOSE_WEB_PORT := $(value GROWTHOS_COMPOSE_WEB_PORT)
compose-identity-maintenance: export GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID := $(value GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID)
compose-identity-maintenance:
	GROWTHOS_COMPOSE_PROJECT="$$COMPOSE_PROJECT" \
	GROWTHOS_COMPOSE_FILE="$$COMPOSE_FILE" \
	GROWTHOS_COMPOSE_WEB_PORT="$$GROWTHOS_COMPOSE_WEB_PORT" \
	GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID="$${GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID:-local-v1}" \
	./scripts/compose-identity-maintenance.sh

compose-smoke:
	GROWTHOS_COMPOSE_PROJECT="$(COMPOSE_PROJECT)" \
	GROWTHOS_COMPOSE_FILE="$(COMPOSE_FILE)" \
	GROWTHOS_COMPOSE_WEB_PORT="$(GROWTHOS_COMPOSE_WEB_PORT)" \
	GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID="$${GROWTHOS_COMPOSE_IDENTITY_CSRF_ACTIVE_KEY_ID:-local-v1}" \
	./scripts/compose-smoke.sh

compose-lottery-api-acceptance:
	./scripts/compose-lottery-api-acceptance.sh

compose-load-health:
	go run ./cmd/healthload \
		-url "http://127.0.0.1:$(GROWTHOS_COMPOSE_WEB_PORT)/health" \
		-rate "$(HEALTHLOAD_RATE)" \
		-duration "$(HEALTHLOAD_DURATION)" \
		-workers "$(HEALTHLOAD_WORKERS)" \
		-timeout "$(HEALTHLOAD_TIMEOUT)" \
		-max-p99 "$(HEALTHLOAD_MAX_P99)"

compose-load-ready:
	go run ./cmd/healthload \
		-url "http://127.0.0.1:$(GROWTHOS_COMPOSE_WEB_PORT)/ready" \
		-rate "$(READYLOAD_RATE)" \
		-duration "$(READYLOAD_DURATION)" \
		-workers "$(HEALTHLOAD_WORKERS)" \
		-timeout "$(HEALTHLOAD_TIMEOUT)"

compose-verify:
	$(MAKE) verify
	$(MAKE) compose-up
	$(MAKE) compose-smoke

compose-m0: compose-up
	$(MAKE) compose-smoke
	go run ./cmd/healthload -url "http://127.0.0.1:$(GROWTHOS_COMPOSE_WEB_PORT)/health" -rate 100 -duration 5m -workers 32 -timeout 2s -max-p99 100ms
	go run ./cmd/healthload -url "http://127.0.0.1:$(GROWTHOS_COMPOSE_WEB_PORT)/ready" -rate 20 -duration 30s -workers 32 -timeout 2s
	$(MAKE) compose-smoke

docs-sync:
	@test -n "$(VAULT)" || (printf '%s\n' '请指定 VAULT，例如 make docs-sync VAULT=/absolute/path/to/growthOS' && exit 1)
	go run ./cmd/docsync --vault "$(VAULT)"

docs-sync-watch:
	@test -n "$(VAULT)" || (printf '%s\n' '请指定 VAULT，例如 make docs-sync-watch VAULT=/absolute/path/to/growthOS' && exit 1)
	go run ./cmd/docsync --vault "$(VAULT)" --watch

verify: fmt-check vet test doc-check web-verify
