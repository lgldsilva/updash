# updash — System Update Dashboard
.PHONY: all build test test-race lint fmt gosec vulncheck clean install check run tidy

BINARY=updash
INSTALL_DIR=$(HOME)/.local/bin

# ── Primary ────────────────────────────────────────────────────────────────

all: build test lint fmt

build:
	go build ./...

test: test-race

test-race:
	go test -race -shuffle=on -count=1 -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1

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
	go run github.com/securego/gosec/v2/cmd/gosec@v2.27.1 -quiet ./...

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# ── Utility ────────────────────────────────────────────────────────────────

install: build
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"

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
