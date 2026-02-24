.PHONY: build test lint run clean setup-hooks bench conformance conformance-all conformance-level-0 conformance-level-1 conformance-level-2 conformance-level-3 conformance-level-4

CONFORMANCE_RUNNER = ../ojs-conformance/runner/http
CONFORMANCE_SUITES = ../../suites
OJS_URL ?= http://localhost:8080

build:
	go build -o bin/ojs-server ./cmd/ojs-server

test:
	go test ./... -race -cover

bench:
	go test ./internal/memory/... -bench=. -benchmem -run='^$$' -benchtime=1000x

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		go vet ./...; \
	fi

run: build
	OJS_ALLOW_INSECURE_NO_AUTH=true ./bin/ojs-server

clean:
	rm -rf bin/

setup-hooks:
	@ln -sf ../../scripts/pre-commit.sh .git/hooks/pre-commit
	@echo "✅ Pre-commit hook installed"

# ── Conformance ──────────────────────────────────

conformance: conformance-all

conformance-all:
	cd $(CONFORMANCE_RUNNER) && go run . -url $(OJS_URL) -suites $(CONFORMANCE_SUITES)

conformance-level-0:
	cd $(CONFORMANCE_RUNNER) && go run . -url $(OJS_URL) -suites $(CONFORMANCE_SUITES) -level 0

conformance-level-1:
	cd $(CONFORMANCE_RUNNER) && go run . -url $(OJS_URL) -suites $(CONFORMANCE_SUITES) -level 1

conformance-level-2:
	cd $(CONFORMANCE_RUNNER) && go run . -url $(OJS_URL) -suites $(CONFORMANCE_SUITES) -level 2

conformance-level-3:
	cd $(CONFORMANCE_RUNNER) && go run . -url $(OJS_URL) -suites $(CONFORMANCE_SUITES) -level 3

conformance-level-4:
	cd $(CONFORMANCE_RUNNER) && go run . -url $(OJS_URL) -suites $(CONFORMANCE_SUITES) -level 4
