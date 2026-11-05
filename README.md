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

## Technology Stack

| Technology | Purpose |
|---|---|
| Go | Primary programming language for backend development |
| Gin | HTTP routing and API handling |
| PostgreSQL | Primary database and source of truth |
| Elasticsearch | Full-text article search and indexing |
| Redis | Optional read caching |
| Goose | Database schema migrations |
| Docker Compose | Local development and service orchestration |
| k6 | Load testing and performance evaluation |
| Postman | Manual API testing |

The application uses direct SQL queries through PostgreSQL drivers without an ORM, as required by the assessment.

## Project Structure

The repository follows a modular structure that separates application responsibilities, infrastructure, deployment configuration, tests, and development documentation.

Each directory has a specific responsibility, making the codebase easier to understand, test, and maintain.

## Design & Development Approach

### Spec-Driven Development (SDD)

This project was developed using **Spec-Driven Development (SDD)**, with AI serving as the primary tool throughout the development process.

I chose this approach because it makes development more structured, consistent, and efficient. Instead of immediately generating code, requirements, technical decisions, implementation tasks, and acceptance criteria are defined before implementation begins.

The development process follows these stages:

1. **Requirement Analysis:** Analyze assessment requirements, constraints, and expected system behavior.
2. **PRD Creation:** Define product requirements and functional expectations.
3. **Technical Design:** Define the architecture, technology stack, data flow, and technical decisions through the Technical Design Document (TDD).
4. **Implementation Planning:** Break down development into manageable tasks with dependencies and clear completion criteria.
5. **Incremental Implementation:** Implement each task progressively based on the approved specifications.
6. **Testing & Verification:** Review generated code, execute unit and integration tests, and verify implementation against the specifications.

The specifications are maintained as **living documents**. When an issue or requirement change is identified, the relevant specification can be updated and tracked through version control before the implementation is adjusted.

This approach provides AI with clear context, rules, and constraints, helping reduce implementation deviations and keeping the development process aligned with the intended requirements.

### Development Documentation

The following documents support the development process:

- Product Requirements Document (PRD): [Article-Service-PRD](./specs/Article-Service-PRD.md)
- Technical Design Document (TDD): [Article-Service-TDD](./specs/Article-Service-TDD.md)
- Implementation Plan: [Implementation Plan](./specs/IMPLEMENTATION.md)
- Architecture Decision Records: [Architecture Decisions](#architecture-decisions)

## Prerequisites

- Go 1.27.1 or compatible.
- Docker Desktop with Docker Compose v2.
- PowerShell on Windows (the commands below use PowerShell syntax).
- Optional: GNU Make for the convenience targets in `Makefile`.
- Optional: `psql`, `curl.exe`, and `k6` for direct database, API, and
  benchmark operations.

The development Compose file starts PostgreSQL 16.9, Elasticsearch 8.17.4,
Redis 7.4.3, the API, and the outbox worker. Elasticsearch security is disabled
and PostgreSQL uses local development credentials; this Compose file is not a
production topology.

## Configuration

Use [.env.example](./.env.example) as a configuration reference. The API and
worker require `POSTGRES_DSN`, `ELASTICSEARCH_URL`, and `REDIS_ADDR`.
The development Compose file supplies these values internally. When running
the binaries directly on the host, use host addresses such as
`localhost:5433`, `localhost:9200`, and `localhost:6379`.

| Variable | Example | Purpose |
|---|---|---|
| `POSTGRES_DSN` | `postgres://app:app@localhost:5433/articles?sslmode=disable` | PostgreSQL connection |
| `POSTGRES_MAX_CONNS` | `20` | Maximum database connections |
| `POSTGRES_MIN_CONNS` | `2` | Minimum idle connections |
| `ELASTICSEARCH_URL` | `http://localhost:9200` | Elasticsearch URL |
| `ELASTICSEARCH_INDEX` | `articles-v1` | Write index |
| `ELASTICSEARCH_USERNAME` | empty locally | Basic Auth username in production |
| `ELASTICSEARCH_PASSWORD` | empty locally | Basic Auth password in production |
| `REDIS_ADDR` | `localhost:6379` | Optional cache address |
| `REDIS_PASSWORD` | empty locally | Redis password; required in production |
| `REDIS_DB` | `0` | Redis logical database |
| `CACHE_TTL` | `45s` | Cache TTL |
| `OUTBOX_BATCH_SIZE` | `100` | Worker batch size |
| `OUTBOX_POLL_INTERVAL` | `1s` | Worker polling interval |
| `OUTBOX_MAX_ATTEMPTS` | `10` | Retry/dead-letter threshold |
| `APP_ENV` | `development` | Environment name |
| `HTTP_ADDR` | `:8080` | API listen address |
| `HTTP_READ_TIMEOUT` | `5s` | HTTP read timeout |
| `HTTP_WRITE_TIMEOUT` | `10s` | HTTP write timeout |
| `HTTP_IDLE_TIMEOUT` | `60s` | HTTP idle timeout |

Inside Compose, use service names `postgres`, `elasticsearch`, and `redis`.
From the host, PostgreSQL is exposed on port `5433`, Elasticsearch on `9200`,
Redis on `6379`, and the API on `8080`.

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

Run from the repository root. Start the dependencies first, apply migrations,
then start the API and worker:

```powershell
docker compose -p article-service -f deployments\docker-compose.yml up -d postgres elasticsearch redis
$env:POSTGRES_DSN = "postgres://app:app@localhost:5433/articles?sslmode=disable"
go run github.com/pressly/goose/v3/cmd/goose -dir migrations postgres $env:POSTGRES_DSN up
docker compose -p article-service -f deployments\docker-compose.yml up -d --build api worker
```

Equivalent Makefile commands are:

```powershell
make docker-up
make docker-ps
```

`make docker-up` starts all services, but migrations still need to be applied
explicitly before the API handles requests.

Check service health:

```powershell
docker compose -p article-service -f deployments\docker-compose.yml ps
curl.exe http://localhost:8080/health
curl.exe http://localhost:8080/ready
```

Migration commands use the `POSTGRES_DSN` environment variable. Set it before
running them:

```powershell
$env:POSTGRES_DSN = "postgres://app:app@localhost:5433/articles?sslmode=disable"
go run github.com/pressly/goose/v3/cmd/goose -dir migrations postgres $env:POSTGRES_DSN up
go run github.com/pressly/goose/v3/cmd/goose -dir migrations postgres $env:POSTGRES_DSN status
```

The equivalent Makefile targets are `make migrate-up`,
`make migrate-status`, and `make migrate-down`.

Stop the local stack with:

```powershell
make docker-down
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

### `POST /authors`

Creates an author record. Author names may be duplicated; author filtering
normalizes names with `LOWER(BTRIM(name))`.

Request:

```json
{
  "name": "Andi Pratama"
}
```

Returns `201 Created`:

```json
{
  "data": {
    "id": 3,
    "name": "Andi Pratama",
    "created_at": "2026-11-04T00:00:00Z"
  }
}
```

The name must not be blank and must not exceed the database limit of 255
characters.

### `GET /authors`

Returns all authors ordered by ascending `id`. Duplicate names are preserved
as separate author records.

Response:

```json
{
  "data": [
    {
      "id": 1,
      "name": "Budi Santoso",
      "created_at": "2026-11-04T00:00:00Z"
    }
  ]
}
```

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

An Elasticsearch search cursor is bound to the exact keyword and author
filter used to create it. For example, a cursor returned by
`GET /articles?query=Golang&limit=10` must be sent to a subsequent request that
also includes `query=Golang` (and the same `author`, if present). Removing or
changing those filters returns `INVALID_CURSOR`.

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

### Request examples

Create an author and then use its ID to create an article:

```powershell
$author = Invoke-RestMethod -Method Post `
  -Uri http://localhost:8080/authors `
  -ContentType "application/json" `
  -Body '{"name":"Andi Pratama"}'

$article = Invoke-RestMethod -Method Post `
  -Uri http://localhost:8080/articles `
  -ContentType "application/json" `
  -Body (@{
    author_id = $author.data.id
    title = "Membangun API Go"
    body = "Contoh artikel untuk pengujian Article Service."
  } | ConvertTo-Json)
```

List, filter, search, paginate, and update articles:

```text
GET   http://localhost:8080/authors
GET   http://localhost:8080/articles?limit=20
GET   http://localhost:8080/articles?author=Andi%20Pratama&limit=20
GET   http://localhost:8080/articles?query=Golang&limit=20
GET   http://localhost:8080/articles?query=Golang&author=Andi%20Pratama&limit=20
GET   http://localhost:8080/articles?query=Golang&limit=20&cursor=<next_cursor>
PATCH http://localhost:8080/articles/<article_id>
GET   http://localhost:8080/health
GET   http://localhost:8080/ready
```

An Elasticsearch cursor must be reused with the same `query` and `author`
parameters that produced it. Changing or removing those filters returns
`INVALID_CURSOR`.

## Tests and validation

From the repository root, use the Makefile or run the commands directly.

Format, static analysis, and build:

```powershell
gofmt -l .
go vet ./...
go build ./...
git diff --check
```

Equivalent targets are `make fmt`, `make vet`, and `make build`.

Unit and package tests:

```powershell
go test -count=1 -p 1 ./...
```

For unit/package tests without the integration package:

```powershell
make test-unit
```

`make test` runs packages serially with `-p 1`; integration tests skip
themselves when their environment variables are not set.

Integration tests require real PostgreSQL and Elasticsearch. Redis-backed tests
also use the real Redis service when `REDIS_ADDR` is set. Start the local
dependencies first:

```powershell
docker compose -p article-service -f deployments\docker-compose.yml up -d postgres elasticsearch redis
```

The PowerShell command below runs the complete suite inside Linux Go on the
Compose network. This avoids Windows Docker-DNS and Windows test-binary
execution restrictions:

```powershell
$repo = (Get-Location).Path
docker run --rm --network article-service_backend `
  -v "${repo}:/src" -w /src `
  -e POSTGRES_DSN="postgres://app:app@deployments-postgres-1:5432/articles?sslmode=disable" `
  -e ELASTICSEARCH_URL="http://elasticsearch:9200" `
  -e REDIS_ADDR="redis:6379" `
  golang:1.27.1-alpine go test -count=1 -p 1 ./...
```

The explicit `-p article-service` above makes the Docker network name
`article-service_backend` predictable for the test command. If the stack was
started with a different Compose project name, replace the network name in the
`docker run` command accordingly.

`-p 1` is intentional because integration packages share a PostgreSQL
database. The integration tests read `POSTGRES_DSN`, `ELASTICSEARCH_URL`, and
`REDIS_ADDR`; if a required variable is absent, the relevant test reports
`SKIP`, not `PASS`. Tests cover migrations, outbox claim/retry/dead-letter
behavior, worker synchronization, Elasticsearch search/PIT, cache behavior,
and API flows.

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

## AI-Assisted Development

### AI as a Development Tool

AI was used throughout the development of this project, including requirements analysis, specification writing, technical design, implementation, test development, documentation, and debugging.

The implementation was AI-generated, guided by the specifications, development rules, and implementation plan defined throughout the project.

I chose to use AI because modern software development is evolving rapidly, and developers should learn to use available tools effectively. AI can accelerate development, reduce repetitive work, and allow developers to focus more on problem-solving, system design, and quality evaluation.

However, AI is a development tool, not a replacement for developer responsibility. The direction, constraints, technical decisions, and final acceptance of the implementation remained under my control.

The responsibilities were divided as follows:

| Activity | AI Contribution | Developer Responsibility |
|---|---|---|
| Requirements & Specifications | Generate and refine specification documents | Define requirements and constraints |
| Technical Design | Suggest architecture and technical solutions | Evaluate and approve technical decisions |
| Implementation | Generate application code | Review and understand the implementation |
| Testing | Generate test cases and test code | Execute tests and evaluate results |
| Debugging | Analyze errors and suggest solutions | Verify issues and determine the final solution |
| Documentation | Generate and organize documentation | Ensure accuracy and consistency with the implementation |

Using SDD helped maintain clear context, rules, and boundaries for AI-generated work. Tasks were implemented incrementally, allowing each stage to be reviewed and verified before proceeding.

Specifications were also maintained as living documents. When an issue was identified, the relevant specification could be updated and tracked through version control before adjusting the implementation.

My guiding principle is:

> AI can generate code, but developers remain responsible for deciding what should be built, why it should be built, how the result should be evaluated, and whether the final implementation is acceptable.

### AI Failure Case

**Case: Incorrectly Generated Nested Go Test Function**

During test development, AI generated a nested Go test function declaration, which is not valid Go syntax.

- **Initial Suggestion:** AI inserted a test function declaration inside another function.
- **Why It Was Incorrect:** Go does not support nested named function declarations.
- **How It Was Identified:** Running the Go test command produced a syntax error: `expected '('`.
- **Final Solution:** I inspected the surrounding test code and moved the test function declarations to package scope.
- **Verification:** I ran Go formatting and the relevant focused tests, followed by the broader test suite.

This case demonstrates that AI-generated code is not always correct. Generated implementations must be reviewed, understood, and validated before being accepted.

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
