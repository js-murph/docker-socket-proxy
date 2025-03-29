.PHONY: build test release lint run release release-docs help

all: lint test build

build:
	goreleaser build --clean --snapshot --single-target

test:
	gotestsum

lint:
	golangci-lint run

run:
	go run cmd/main.go daemon

version:
	@echo "Creating a new version..."
	@echo "Current version: $$(svu current)"
	@echo "Next version: $$(svu next)"
	@if [ "$$CI" = "true" ]; then \
		echo "Creating git tag for version $$(svu next)"; \
		git tag -a $$(svu next) -m "Release $$(svu next)"; \
		git push origin $$(svu next); \
	else \
		read -p "Continue with this version? [y/N] " confirm; \
		if [ "$$confirm" = "y" ] || [ "$$confirm" = "Y" ]; then \
			echo "Creating git tag for version $$(svu next)"; \
			git tag -a $$(svu next) -m "Release $$(svu next)"; \
			git push origin $$(svu next); \
		else \
			echo "Release cancelled"; \
		fi \
	fi

release:
	@echo "Creating a new release for $$(svu current)..."
	@if [ "$$CI" = "true" ]; then \
		echo "Running goreleaser..."; \
		goreleaser release --clean; \
	else \
		read -p "Continue with this version? [y/N] " confirm; \
		if [ "$$confirm" = "y" ] || [ "$$confirm" = "Y" ]; then \
			echo "Running goreleaser..."; \
			goreleaser release --clean; \
		else \
			echo "Release cancelled"; \
		fi \
	fi

release-docs:
	$(MAKE) -C docs release


help:
	@echo "Usage: make <target>"
	@echo "Targets:"
	@echo "  build: Build the project"
	@echo "  test: Run the tests"
	@echo "  lint: Run the linter"
	@echo "  run: Run the project"
	@echo "  release: Release the project"
	@echo "  help: Show this help message"
