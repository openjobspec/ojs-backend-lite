.PHONY: build test lint run clean setup-hooks bench

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
