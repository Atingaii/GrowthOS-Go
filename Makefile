.PHONY: help fmt fmt-check test doc-check docs-sync verify

help:
	@printf '%s\n' \
		'Available targets:' \
		'  make fmt        Format Go code' \
		'  make fmt-check  Fail when Go code is not formatted' \
		'  make test       Run Go tests' \
		'  make doc-check  Check documentation integrity and course evidence' \
		'  make verify     Run all local quality gates'

fmt:
	gofmt -w $$(find . -type f -name '*.go' -not -path './vendor/*')

fmt-check:
	@test -z "$$(gofmt -l $$(find . -type f -name '*.go' -not -path './vendor/*'))" || \
		(printf '%s\n' 'Go files require formatting. Run make fmt.' && exit 1)

test:
	go test ./...

doc-check:
	go run ./cmd/doccheck

docs-sync:
	@test -n "$(VAULT)" || (printf '%s\n' '请指定 VAULT，例如 make docs-sync VAULT=/mnt/e/TencentGo/growthOS' && exit 1)
	go run ./cmd/docsync --vault "$(VAULT)"

verify: fmt-check test doc-check
