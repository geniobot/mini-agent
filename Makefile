BINARY  := mini-agent
BIN_DIR := ./bin
PREFIX  ?= /usr/local

.PHONY: build install uninstall clean run test

build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "-s -w" -o $(BIN_DIR)/$(BINARY) ./cmd/$(BINARY)

install: build
	install -Dm755 $(BIN_DIR)/$(BINARY) $(PREFIX)/bin/$(BINARY)
	@echo "Installed to $(PREFIX)/bin/$(BINARY)"

uninstall:
	rm -f $(PREFIX)/bin/$(BINARY)

clean:
	rm -f $(BIN_DIR)/$(BINARY)

run:
	go run ./cmd/$(BINARY)/

test:
	go test ./...

.DEFAULT_GOAL := build
