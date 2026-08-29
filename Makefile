.PHONY: help fmt fmt-check vet test test-race api-run doc-check web-install web-typecheck web-build web-verify docs-sync docs-sync-watch verify

help:
	@printf '%s\n' \
		'Available targets:' \
		'  make fmt        Format Go code' \
		'  make fmt-check  Fail when Go code is not formatted' \
		'  make vet        Run Go static analysis' \
		'  make test       Run Go tests' \
		'  make test-race  Run Go tests with the race detector' \
		'  make api-run    Run the GrowthOS API locally' \
		'  make doc-check  Check documentation integrity and course evidence' \
		'  make web-verify Run frontend typecheck and production build' \
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

doc-check:
	go run ./cmd/doccheck

web-install:
	cd web && pnpm install --frozen-lockfile

web-typecheck:
	cd web && pnpm run typecheck

web-build:
	cd web && pnpm run build

web-verify: web-typecheck web-build

docs-sync:
	@test -n "$(VAULT)" || (printf '%s\n' '请指定 VAULT，例如 make docs-sync VAULT=/absolute/path/to/growthOS' && exit 1)
	go run ./cmd/docsync --vault "$(VAULT)"

docs-sync-watch:
	@test -n "$(VAULT)" || (printf '%s\n' '请指定 VAULT，例如 make docs-sync-watch VAULT=/absolute/path/to/growthOS' && exit 1)
	go run ./cmd/docsync --vault "$(VAULT)" --watch

verify: fmt-check vet test doc-check web-verify
