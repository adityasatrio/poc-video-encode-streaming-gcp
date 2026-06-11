.PHONY: test test-verbose test-coverage build clean help

help:
	@echo "Available targets:"
	@echo "  make test           - Run all tests"
	@echo "  make test-verbose   - Run tests with verbose output"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make build          - Build the binary"
	@echo "  make clean          - Remove build artifacts"

test:
	@go test ./... -race

test-verbose:
	@go test ./... -v -race

test-coverage:
	@go test ./... -race -coverprofile=coverage.out -covermode=atomic
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

build:
	@CGO_ENABLED=0 GOOS=linux go build -o server .

clean:
	@rm -f server coverage.out coverage.html
	@go clean -testcache
