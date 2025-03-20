.PHONY: build test release lint

all: lint test build

build:
	goreleaser build --clean --snapshot --single-target

test:
	gotestsum

lint:
	golangci-lint run

release:
#	TODO: Add release steps
