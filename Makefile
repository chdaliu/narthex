BINARY := build/narthex
VERSION ?= $(shell (git describe --tags --abbrev=0 2>/dev/null || echo v0.0.1) | sed 's/^v//')
LDFLAGS := -X main.version=$(VERSION)

.PHONY: build run setup passwd smoke tidy clean

build:
	mkdir -p build
	go build -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/narthex

run:
	go run ./cmd/narthex serve

setup:
	go run ./cmd/narthex setup

passwd:
	go run ./cmd/narthex passwd

smoke:
	./scripts/smoke-test.sh

tidy:
	go mod tidy

clean:
	rm -rf build