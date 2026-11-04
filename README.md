# Article Service

Article Service is a Go modular monolith for creating, updating, listing, and
searching articles. PostgreSQL is the source of truth. Elasticsearch is a
derived search projection updated through a transactional outbox and worker.
Redis is an optional read cache.

## Architecture

```text
HTTP client
    |
    v
Gin API -----> PostgreSQL (source of truth)
    |                 |
    |                 +--> transactional outbox
    |                              |
    |                              v
    +--> Redis cache          outbox worker --> Elasticsearch
    |
    +--> Elasticsearch search --> PostgreSQL hydration
```

Important behavior:

- Article writes and outbox events commit in the same PostgreSQL transaction.
- Search results are eventually consistent with PostgreSQL.
- PostgreSQL listing uses keyset cursors.
- Elasticsearch search uses PIT and `search_after` cursors.
- Search results are hydrated from PostgreSQL and returned in Elasticsearch
  relevance order.
- Redis failures fall back to the underlying PostgreSQL/Elasticsearch path.
- Author matching is exact after `LOWER(BTRIM(name))`; accents are not folded.
- `created_at` is immutable; updates increment `version`.

## Prerequisites

- Go 1.27.1 or compatible.
- Docker and Docker Compose.
- Optional: `psql`, `k6`, and `curl` for direct database, benchmark, and API
  operations.

The local Compose file pins PostgreSQL 16.9, Elasticsearch 8.17.4, and Redis
7.4.3. Elasticsearch is intentionally configured as a single-node local
development dependency and is not a production HA topology.

## Configuration

Use [.env.example](./.env.example) as a configuration reference and adjust
values for the execution environment. Required settings include:

| Variable | Example | Purpose |
|---|---|---|
| `POSTGRES_DSN` | `postgres://app:app@localhost:5433/articles?sslmode=disable` | PostgreSQL connection |
| `POSTGRES_MAX_CONNS` | `20` | Maximum database connections |
| `POSTGRES_MIN_CONNS` | `2` | Minimum idle connections |
| `ELASTICSEARCH_URL` | `http://localhost:9200` | Elasticsearch URL |
| `ELASTICSEARCH_INDEX` | `articles-v1` | Write index |
| `REDIS_ADDR` | `localhost:6379` | Optional cache address |
| `CACHE_TTL` | `45s` | Cache TTL |
| `OUTBOX_BATCH_SIZE` | `100` | Worker batch size |
| `OUTBOX_POLL_INTERVAL` | `1s` | Worker polling interval |
| `OUTBOX_MAX_ATTEMPTS` | `10` | Retry/dead-letter threshold |

The Compose service names (`postgres`, `elasticsearch`, and `redis`) must be
used from containers. The host PostgreSQL port is `5433`; the API port is
`8080`.

### Production configuration

The development Compose file deliberately disables Elasticsearch security and
uses local-only credentials. It must not be used as a production deployment.
For production, use the separate
[docker-compose.production.yml](./deployments/docker-compose.production.yml):

```powershell
$env:POSTGRES_DSN = "postgres://app:$($env:POSTGRES_PASSWORD)@postgres.example:5432/articles?sslmode=verify-full"
$env:ELASTICSEARCH_URL = "https://elasticsearch.example:9243"
$env:ELASTICSEARCH_USERNAME = "article-service"
$env:ELASTICSEARCH_PASSWORD = "<inject-from-secret-manager>"
$env:REDIS_ADDR = "rediss://redis.example:6380"
$env:REDIS_PASSWORD = "<inject-from-secret-manager>"
docker compose -f deployments\docker-compose.production.yml config
docker compose -f deployments\docker-compose.production.yml up -d --build
```

Do not put the values above in Git, a committed `.env` file, image layers, or
logs. Inject them from the deployment platform's secret manager. Production
configuration validation rejects plain HTTP Elasticsearch, disabled PostgreSQL
TLS, missing Elasticsearch credentials, and missing Redis password.

The production Compose file assumes managed PostgreSQL, Elasticsearch, Redis,
TLS certificates, an ingress/load balancer, backups, and an external identity
provider. It does not provision those dependencies or provide authentication
itself.

Before the first application start, run migrations as a controlled release
step:

```powershell
go run github.com/pressly/goose/v3/cmd/goose -dir migrations postgres $env:POSTGRES_DSN up
```

For rollback, stop new application releases, verify the migration is
backward-compatible, then run `goose ... down` for the last migration and
restore the prior application version. Do not use `down` as an emergency
data-loss operation without a verified backup.

Back up PostgreSQL with the platform's encrypted, tested backup service (for
example, a managed PostgreSQL snapshot plus point-in-time recovery). Verify
restore regularly. Elasticsearch is a rebuildable projection: preserve the
PostgreSQL source and replay the outbox or run the index bootstrap/reindex
procedure before directing search traffic to a rebuilt index. Redis is
disposable and may be flushed/recreated.

Deployment rollback is image-based: retain the previous immutable image,
rollback API and worker together, and keep migrations backward-compatible.
Confirm `/ready`, outbox progress, and search freshness before reopening
traffic.

## Run locally with Docker Compose

From the repository root:

```powershell
docker compose -f deployments\docker-compose.yml up -d --build
```

Check service health:

```powershell
docker compose -f deployments\docker-compose.yml ps
curl.exe http://localhost:8080/health
curl.exe http://localhost:8080/ready
```

Run migrations explicitly before starting the API and worker. To run Goose
commands directly from the host, set `POSTGRES_DSN` first:

```powershell
$env:POSTGRES_DSN = "postgres://app:app@localhost:5433/articles?sslmode=disable"
go run github.com/pressly/goose/v3/cmd/goose -dir migrations postgres $env:POSTGRES_DSN up
go run github.com/pressly/goose/v3/cmd/goose -dir migrations postgres $env:POSTGRES_DSN status
```

Migration rollback and re-apply:

```powershell
go run github.com/pressly/goose/v3/cmd/goose -dir migrations postgres $env:POSTGRES_DSN down
go run github.com/pressly/goose/v3/cmd/goose -dir migrations postgres $env:POSTGRES_DSN up
```

## Seed data

The small development seed inserts named authors:

```powershell
$env:POSTGRES_DSN = "postgres://app:app@localhost:5433/articles?sslmode=disable"
go run ./scripts/seed
```

For configurable benchmark data:

```powershell
go run ./scripts/benchmark -dsn $env:POSTGRES_DSN `
  -articles 10000 -authors 100 -batch 500 -seed 19
```

The benchmark seed assigns approximately one third of articles to the
dominant author and distributes the remainder over long-tail authors.

## API contract

All endpoints use `application/json`.

### `POST /articles`

Request:

```json
{
  "author_id": 1,
  "title": "Introduction to Golang",
  "body": "Golang is a programming language..."
}
```

Returns `201 Created` with `{ "data": { ... } }`. `author_id` must be positive,
the author must exist, title/body must be non-blank, title is limited to 500
characters, body to 50,000 characters, and the request body to 2 MB.

### `GET /articles`

Query parameters:

| Parameter | Default | Description |
|---|---:|---|
| `query` | absent | Keyword search over title/body |
| `author` | absent | Exact normalized author name |
| `limit` | `20` | Integer from 1 to 100 |
| `cursor` | absent | Opaque cursor bound to its query/filter |

Examples:

```text
GET /articles
GET /articles?author=Budi&limit=20
GET /articles?query=golang
GET /articles?query=golang&author=Budi
GET /articles?query=golang&cursor=<opaque-token>
```

Response:

```json
{
  "data": [],
  "pagination": {
    "limit": 20,
    "next_cursor": null,
    "has_more": false
  }
}
```

### `PATCH /articles/{id}`

Request:

```json
{
  "title": "Updated title",
  "body": "Updated body"
}
```

At least one field is required. Omitted fields are retained; explicitly blank
fields are rejected. `created_at` remains unchanged, `updated_at` changes on
success, and `version` increments.

### Operational endpoints

- `GET /health` returns `200` while the process is alive.
- `GET /ready` returns `200` only when PostgreSQL is reachable; otherwise
  `503`.
- Successful and error responses use the documented `data`/`error` envelopes.
- Errors do not expose SQL, Elasticsearch, credentials, or other internal
  infrastructure details.
- Every response includes `X-Request-ID`.

There is no generated OpenAPI file in this repository; the contract above is
the current API reference and is covered by handler/integration tests.

## Tests and validation

Format, static analysis, and build:

```powershell
gofmt -l .
go vet ./...
go build ./...
git diff --check
```

Unit and package tests:

```powershell
go test -count=1 -p 1 ./...
```

Integration tests require the real PostgreSQL, Elasticsearch, and Redis
services. The validated Docker-network form is:

```powershell
$repo = (Get-Location).Path
docker run --rm --network deployments_backend `
  -v "${repo}:/src" -w /src `
  -e POSTGRES_DSN="postgres://app:app@deployments-postgres-1:5432/articles?sslmode=disable" `
  -e ELASTICSEARCH_URL="http://elasticsearch:9200" `
  -e REDIS_ADDR="redis:6379" `
  golang:1.27.1-alpine go test -count=1 -p 1 ./...
```

`-p 1` is intentional because integration packages share a PostgreSQL
database. Tests cover migrations, outbox claim/retry/dead-letter behavior,
worker synchronization, Elasticsearch search/PIT, cache behavior, and API
flows.

## Benchmark

The reproducible k6 script is
[article_service.js](./scripts/benchmark/article_service.js). It includes
listing, dominant-author, long-tail-author, keyword, combined filter, cache
hit, cache miss, bursty concurrent reads, and mixed read/write scenarios.

Run it against a running API:

```powershell
$env:BASE_URL = "http://localhost:8080"
k6 run scripts/benchmark/article_service.js
```

The benchmark can also run inside a Docker network with a pinned k6 image
chosen by the execution environment. Set `BASE_URL` to an address reachable
from that container. Results are reported by k6; do not treat local results as
production capacity claims.

Run query plans with:

```powershell
$env:PGPASSWORD = "app"
psql -h localhost -p 5433 -U app -d articles `
  -f scripts/benchmark/explain.sql
```

The actual benchmark run used 1,000 articles and 20 authors, with 33.4% of
articles assigned to the dominant author. Its aggregate result was p50
`0.922565 ms`, p95 `1.9767635 ms`, p99 `20.2779695 ms`, throughput
`28.3527247 req/s`, and error rate `0`. These values are local measurements,
not SLOs.

## Architecture decisions

The eight TDD ADRs are documented individually:

- [ADR-001](./docs/adr/ADR-001-rest-api-and-gin.md)
- [ADR-002](./docs/adr/ADR-002-postgresql-source-of-truth.md)
- [ADR-003](./docs/adr/ADR-003-elasticsearch-search.md)
- [ADR-004](./docs/adr/ADR-004-transactional-outbox.md)
- [ADR-005](./docs/adr/ADR-005-redis-cache.md)
- [ADR-006](./docs/adr/ADR-006-cursor-pagination.md)
- [ADR-007](./docs/adr/ADR-007-duplicate-author-names.md)
- [ADR-008](./docs/adr/ADR-008-mixed-language-analysis.md)

Approved project decisions D-01 through D-06 are recorded in
[IMPLEMENTATION.md](./specs/IMPLEMENTATION.md) and summarized in
[decisions.md](./docs/decisions.md).

## AI-assisted development and verification

Development was assisted by the Copilot SDK in VS Code. The tools used during
the implementation included repository search and file inspection, patch
editing, Git status/diff inspection, Go formatting/build/vet/test commands,
Docker Compose/container execution, PostgreSQL `EXPLAIN`, Elasticsearch and
Redis integration tests, and k6 execution.

AI assistance covered implementation drafts, test drafts, repository
exploration, and validation command selection. Results were verified by
reading the existing code/specifications, preserving the existing working
tree, running focused tests, running the serial full suite, and exercising
real PostgreSQL, Elasticsearch, Redis, and k6 where applicable.

One observed AI error was an incorrectly inserted nested Go test function,
which caused `expected '('` syntax errors. The surrounding function was
inspected, the declarations were moved to package scope, and formatting plus
focused/full tests were rerun. This is why generated changes are not treated
as verified until the repository checks pass.

## Known limitations

- The local benchmark used 1,000 articles, not the 50-million-article
  assessment target. No 50-million capacity claim is made.
- Local Elasticsearch uses one node and no replicas.
- Search is eventually consistent after create/update because Elasticsearch
  is populated asynchronously by the outbox worker.
- Redis uses short TTL and namespace versioning rather than broad key
  enumeration/invalidation.
- Metrics are process-local atomic counters; no Prometheus exposition endpoint
  is included.
- Search cursors are opaque and cannot be reused across changed query/filter
  context.
- The repository has no generated OpenAPI document.
- There is no application authentication or authorization middleware. An API
  gateway or identity-aware ingress is required before exposing write
  endpoints.
- The repository does not include a cloud secret manager, managed database
  backup, certificate distribution, ingress, or alerting configuration.

## License and assessment scope

This repository is an assessment project. No production deployment is
implied by the local Compose configuration or benchmark results.
