.PHONY: build test release lint run

all: lint test build

build:
	goreleaser build --clean --snapshot --single-target

test:
	gotestsum

lint:
	golangci-lint run

run:
	go run cmd/main.go daemon

release:
#	TODO: Add release steps
