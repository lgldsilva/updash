# updash — System Update Dashboard
.PHONY: build install clean run check all

BINARY=updash
INSTALL_DIR=$(HOME)/.local/bin

build:
	go build -o $(BINARY) ./cmd/updash/

install: build
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"

clean:
	rm -f $(BINARY)

run: build
	./$(BINARY)

check: build
	./$(BINARY) --check

all: build
	./$(BINARY) --all

tidy:
	go mod tidy
	go vet ./...
