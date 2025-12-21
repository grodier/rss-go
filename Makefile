# ==================================================================================== #
# HELPERS
# ==================================================================================== #

## help: Show this help message
.PHONY: help
help:
	@echo "Usage:"
	@sed -n 's/^##//p' Makefile | column -t -s ':' |  sed -e 's/^/ /'

.PHONY: confirm
confirm:
	@echo -n "Are you sure? [y/N] " && read ans && [ $${ans:-N} = y ]

# ==================================================================================== #
# DEVELOPMENT
# ==================================================================================== #

## run/api: run the cmd/api application
.PHONY: run/web
run/web:
	go run ./cmd/web -port 8080

# ==================================================================================== #
# QUALITY CONTROL
# ==================================================================================== #

## tidy: format all .go files and tidy module dependencies
.PHONY: tidy
tidy:
	@echo "Formatting all .go files..."
	go fmt ./...
	@echo "Tidying module dependencies..."
	go mod tidy

## test: run all tests
.PHONY: test
test:
	@echo "Running tests..."
	go test -v -race -buildvcs ./...

## test/cover: run all tests with coverage
.PHONY: test/cover
test/cover:
	@echo "Running tests with coverage..."
	go test -v -race -buildvcs -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out

## audit: run quality control checks
.PHONY: audit
audit:
	@echo "Checking module dependencies..."
	go mod tidy -diff
	go mod verify
	@echo "Vetting code..."
	go vet ./...
	@echo "Running tests..."
	go test -race -vet=off ./...
