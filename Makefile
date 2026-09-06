BINARY := build/narthex

.PHONY: build run setup passwd smoke tidy clean

build:
	mkdir -p build
	go build -o $(BINARY) ./cmd/narthex

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