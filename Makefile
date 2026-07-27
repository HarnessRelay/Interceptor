.PHONY: build fmt install run test uninstall update

build:
	npm --prefix web run build
	go build -o bin/harnessd ./cmd/harnessd
	go build -o bin/harnessctl ./cmd/harnessctl

install:
	./scripts/install.sh

update:
	./scripts/install.sh --update
	harnessctl shims reshim
	harnessctl services restart

uninstall:
	./scripts/uninstall.sh

fmt:
	gofmt -w ./cmd ./internal

run:
	go run ./cmd/harnessd serve

test:
	go test ./...
	./scripts/install_test.sh
