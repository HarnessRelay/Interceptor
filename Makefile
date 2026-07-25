.PHONY: build fmt run test

build:
	go build -o bin/harnessd ./cmd/harnessd
	go build -o bin/harnessctl ./cmd/harnessctl

fmt:
	gofmt -w ./cmd ./internal

run:
	go run ./cmd/harnessd serve

test:
	go test ./...

