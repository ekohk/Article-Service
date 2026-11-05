.PHONY: migrate-up migrate-down migrate-status build test test-unit vet fmt \
	docker-up docker-down docker-ps

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
	go fmt ./...

# Run unit/package tests without the integration package.
test-unit:
	go test -count=1 -p 1 ./internal/... ./scripts/...

# Run all Go tests. Integration tests skip themselves when dependencies are absent.
PKG ?= ./...
test:
	go test -count=1 -p 1 $(PKG)

docker-up:
	docker compose -f deployments\docker-compose.yml up -d --build

docker-down:
	docker compose -f deployments\docker-compose.yml down

docker-ps:
	docker compose -f deployments\docker-compose.yml ps
