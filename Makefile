# updash — System Update Dashboard
.PHONY: all build test test-race lint fmt gosec vulncheck clean install check run tidy

BINARY=updash
INSTALL_DIR=$(HOME)/.local/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

# ── Primary ────────────────────────────────────────────────────────────────

# Coverage gate packages (≥90%) — keep in sync with .ai-standards.env and ci.yml
COVER_PKGS=./internal/model/... ./internal/config/... ./internal/sizefmt/... ./internal/cli/... ./internal/retention/... ./internal/upgrade/...

all: build test lint fmt

build:
	go build -ldflags='$(LDFLAGS)' ./...

build-release:
	go build -trimpath -ldflags='-s -w $(LDFLAGS)' -o $(BINARY) ./cmd/updash/

test: test-race

test-race:
	go test -race -shuffle=on -count=1 ./...

test-gate:
	go test -race -shuffle=on -count=1 -coverprofile=coverage.out $(COVER_PKGS)
	@go tool cover -func=coverage.out | tail -1

coverage: test-gate
	@go tool cover -func=coverage.out

test-short:
	go test -race -shuffle=on -short -count=1 ./...

lint:
	go vet ./...
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run ./...

fmt:
	gofmt -l . | tee /dev/stderr | test ! -s /dev/stdin

fmt-fix:
	gofmt -w .

gosec:
	go run github.com/securego/gosec/v2/cmd/gosec@v2.27.1 -quiet -exclude=G204,G306,G703,G118 ./...

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# ── Utility ────────────────────────────────────────────────────────────────

install: build-release
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(BINARY) v$(VERSION) to $(INSTALL_DIR)/$(BINARY)"

clean:
	rm -f $(BINARY) coverage.out

run: build
	./$(BINARY)

check: build
	./$(BINARY) --check

all-headless: build
	./$(BINARY) --all

tidy:
	go mod tidy
	go vet ./...

# ── Help ───────────────────────────────────────────────────────────────────

help:
	@echo "updash — System Update Dashboard"
	@echo ""
	@echo "Primary:"
	@echo "  make all          Build, test, and lint (default)"
	@echo "  make build        Compile all packages"
	@echo "  make test         Run all tests with race detector"
	@echo "  make test-gate    Run gate packages with coverage (≥90%)"
	@echo "  make coverage     Show full coverage report"
	@echo "  make lint         Run golangci-lint"
	@echo ""
	@echo "Quality gates:"
	@echo "  make fmt          Check formatting"
	@echo "  make fmt-fix      Apply gofmt fixes"
	@echo "  make gosec        Run gosec security scanner"
	@echo "  make vulncheck    Run govulncheck"
	@echo ""
	@echo "Utility:"
	@echo "  make install      Build + copy to ~/.local/bin"
	@echo "  make check        Build + --check (headless scan)"
	@echo "  make clean        Remove build artifacts"
