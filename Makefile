.PHONY: migrate-up migrate-down migrate-status build test vet fmt

MIGRATIONS_DIR := ./migrations
GOOSE         := go run github.com/pressly/goose/v3/cmd/goose

# Run migrations forward.
migrate-up:
	$(GOOSE) -dir $(MIGRATIONS_DIR) postgres "$(POSTGRES_DSN)" up

# Roll back the last applied migration.
migrate-down:
	$(GOOSE) -dir $(MIGRATIONS_DIR) postgres "$(POSTGRES_DSN)" down

# Show migration status.
migrate-status:
	$(GOOSE) -dir $(MIGRATIONS_DIR) postgres "$(POSTGRES_DSN)" status

build:
	go build ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

# Run all tests. Uses pre-compiled binaries to work around Application Control
# policies that block test binaries in %TEMP%.
# Usage: make test PKG=./internal/...
PKG ?= ./...
test:
	go test -count=1 $(PKG)
