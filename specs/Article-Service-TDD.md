# Technical Design Document (TDD)

## Article Service --- Backend Engineer Assessment

**Document version:** 1.0.0\
**Status:** Proposed --- requires technical review before
implementation\
**Related document:** Product Requirements Document (PRD) v1.0.0\
**Primary language:** Go\
**API protocol:** REST / JSON\
**Architecture:** Modular Monolith\
**Development approach:** Spec-Driven Development (SDD)

------------------------------------------------------------------------

## 1. Purpose and design principles

This document translates the product requirements into an implementable
technical design. It defines the architecture, data model, API
contracts, search behavior, consistency model, caching, operational
concerns, and validation strategy.

### 1.1 Design principles

1.  PostgreSQL is the system of record. Article writes are committed
    there first.
2.  Elasticsearch is a derived search projection, not a source of truth.
3.  Redis is an optional performance layer. Cache failure must not make
    primary data unavailable.
4.  All database access uses parameterized SQL. No ORM is permitted.
5.  Keep the application a modular monolith; do not introduce
    microservices without evidence that they are needed.
6.  Prefer correctness and measurable performance over premature
    optimization.
7.  Every asynchronous operation must be retryable, observable, and
    idempotent.
8.  AI-generated code must be reviewed and tested against the
    specifications.

### 1.2 Scope

Required: - `POST /articles` - `GET /articles` - Newest-first ordering -
Keyword search across title and body - Author-name filtering, including
duplicate author names - Mixed Indonesian and English content -
High-volume/read-heavy design - SQL-based persistence and tests

Supporting implementation: - `PATCH /articles/{id}` because articles can
be edited. - Transactional outbox and background indexing worker. -
Redis cache. - Health and readiness endpoints. - Docker Compose and
performance test scripts.

Out of scope for this assessment: - Authentication and authorization -
Author CRUD API - Article deletion - Categories, comments, likes,
bookmarks - Frontend application - Recommendation features

------------------------------------------------------------------------

## 2. Architecture overview

### 2.1 Logical architecture

``` mermaid
flowchart TB
    C[API Consumer] --> H[HTTP API - Gin]
    H --> S[Application Services]
    S --> R[(Redis Cache)]
    S --> P[(PostgreSQL - Source of Truth)]
    S --> E[(Elasticsearch - Search Projection)]
    P --> O[Outbox Events]
    O --> W[Outbox Worker]
    W --> E
    W --> R
```

### 2.2 Component responsibilities

  -----------------------------------------------------------------------
  Component               Responsibility          Must not do
  ----------------------- ----------------------- -----------------------
  HTTP handler            Parse requests,         Contain SQL or business
                          validate                rules
                          transport-level input,  
                          map errors to HTTP      

  Application service     Coordinate use cases    Depend on Gin request
                          and business rules      objects

  Domain                  Define article concepts Depend on
                          and invariants          infrastructure clients

  PostgreSQL repository   Execute parameterized   Implement HTTP behavior
                          SQL and transactions    

  Search repository       Execute Elasticsearch   Become the source of
                          queries and indexing    article truth
                          operations              

  Cache repository        Read/write cache        Be required for
                          entries and             correctness
                          invalidation            

  Outbox worker           Claim and process       Lose failed events
                          pending events          silently

  Configuration           Load and validate       Contain secrets in
                          environment             source code
                          configuration           
  -----------------------------------------------------------------------

### 2.3 Request paths

**Create article** 1. Receive and validate request. 2. Start PostgreSQL
transaction. 3. Verify the author exists. 4. Insert article. 5. Insert
`article.created` outbox event in the same transaction. 6. Commit. 7.
Return the committed article with HTTP 201. 8. Worker later indexes the
document in Elasticsearch and invalidates relevant cache entries.

**List without keyword** 1. Normalize and validate filters/pagination.
2. Check Redis cache. 3. On cache miss, query PostgreSQL using indexed
keyset pagination. 4. Cache the response when cache policy allows. 5.
Return results.

**Search with keyword** 1. Normalize and validate filters/pagination. 2.
Check Redis cache. 3. Search Elasticsearch using keyword and optional
author-name filter. 4. Receive ordered article IDs and sort values. 5.
Fetch current article rows from PostgreSQL in one batch. 6. Reconstruct
the Elasticsearch order. 7. Apply the documented
stale/deleted/missing-document policy. 8. Cache the response when safe.
9. Return results.

**Update article** 1. Validate article ID and patch fields. 2. Start
PostgreSQL transaction. 3. Update title/body and `updated_at`; do not
modify `created_at`. 4. Insert `article.updated` outbox event in the
same transaction. 5. Commit. 6. Return the committed row. 7. Worker
updates Elasticsearch and invalidates relevant cache entries.

------------------------------------------------------------------------

## 3. Technology choices

  -----------------------------------------------------------------------
  Concern                 Technology              Reason
  ----------------------- ----------------------- -----------------------
  Language                Go                      Required by assessment

  HTTP                    Gin                     Lightweight REST
                                                  framework

  PostgreSQL access       pgx or `database/sql`   Explicit SQL, no ORM

  Primary database        PostgreSQL              Relational integrity
                                                  and transactional
                                                  writes

  Search                  Elasticsearch           Full-text indexing and
                                                  search

  Cache                   Redis                   Reduce repeated
                                                  read/search work

  Migration               Goose                   Versioned SQL
                                                  migrations

  Tests                   Go `testing`, Testify   Unit and integration
                                                  tests

  Load testing            k6                      Repeatable load
                                                  scenarios

  Local infrastructure    Docker Compose          Reproducible local
                                                  environment
  -----------------------------------------------------------------------

Pin dependency and container versions. Verify Elasticsearch
server/client compatibility before implementation. Do not use floating
`latest` tags in reproducible assessment environments.

------------------------------------------------------------------------

## 4. Repository structure

``` text
article-service/
├── cmd/
│   ├── api/
│   │   └── main.go
│   └── worker/
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── domain/
│   │   └── article/
│   │       ├── entity.go
│   │       ├── errors.go
│   │       └── repository.go
│   ├── application/
│   │   └── article/
│   │       ├── service.go
│   │       ├── dto.go
│   │       └── service_test.go
│   ├── handler/
│   │   └── http/
│   │       ├── article_handler.go
│   │       ├── health_handler.go
│   │       ├── response.go
│   │       └── router.go
│   ├── repository/
│   │   ├── postgres/
│   │   │   ├── article_repository.go
│   │   │   ├── author_repository.go
│   │   │   └── outbox_repository.go
│   │   ├── elasticsearch/
│   │   │   ├── article_search.go
│   │   │   └── mapping.go
│   │   └── redis/
│   │       └── article_cache.go
│   ├── worker/
│   │   └── outbox/
│   │       ├── worker.go
│   │       └── worker_test.go
│   └── platform/
│       ├── postgres/
│       │   └── connection.go
│       ├── elasticsearch/
│       │   └── connection.go
│       └── redis/
│           └── connection.go
├── migrations/
├── scripts/
│   ├── seed/
│   └── benchmark/
├── tests/
│   └── integration/
├── specs/
├── docs/
├── deployments/
│   └── docker-compose.yml
├── .env.example
├── .gitignore
├── Dockerfile
├── Makefile
├── go.mod
└── README.md
```

Keep interfaces close to the domain/application consumer. Avoid creating
interfaces for every concrete type unless they enable testing or isolate
a real dependency.

------------------------------------------------------------------------

## 5. Domain model and business rules

### 5.1 Article

``` go
type Article struct {
    ID        int64     `json:"id"`
    AuthorID  int64     `json:"author_id"`
    Title     string    `json:"title"`
    Body      string    `json:"body"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

### 5.2 Author

``` go
type Author struct {
    ID        int64     `json:"id"`
    Name      string    `json:"name"`
    CreatedAt time.Time `json:"created_at"`
}
```

### 5.3 Business rules

-   `authors.id` is unique; `authors.name` is not unique.
-   Author filtering is exact-name matching after trimming outer
    whitespace and case normalization.
-   All author IDs with a matching normalized name are included.
-   Partial author-name matching is not supported.
-   Article title and body are required and must not be blank.
-   `created_at` is immutable after insert.
-   `updated_at` changes on a successful update.
-   Article ordering is deterministic: `created_at DESC, id DESC`.
-   Pagination limits are validated and bounded.
-   PostgreSQL is authoritative when a search document is stale or
    missing.

------------------------------------------------------------------------

## 6. PostgreSQL design

### 6.1 Schema

The original assessment does not provide a complete column-level schema.
The following is the proposed schema for this implementation.

``` sql
CREATE TABLE authors (
    id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE articles (
    id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    author_id BIGINT NOT NULL REFERENCES authors(id),
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT articles_title_not_blank CHECK (BTRIM(title) <> ''),
    CONSTRAINT articles_body_not_blank CHECK (BTRIM(body) <> '')
);

CREATE TABLE outbox_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id BIGINT NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ,
    last_error TEXT
);
```

The assessment only requires article and author relationships.
`outbox_events` is an implementation support table.

### 6.2 Indexes

``` sql
-- Global newest-first listing and keyset pagination.
CREATE INDEX idx_articles_created_id
    ON articles (created_at DESC, id DESC);

-- Author-scoped listing and ordering.
CREATE INDEX idx_articles_author_created_id
    ON articles (author_id, created_at DESC, id DESC);

-- Case-insensitive exact author-name lookup.
CREATE INDEX idx_authors_lower_name
    ON authors (LOWER(name));

-- Pending outbox work. A partial index keeps processed rows out.
CREATE INDEX idx_outbox_pending
    ON outbox_events (available_at, id)
    WHERE processed_at IS NULL;
```

Do not add indexes speculatively. Measure write overhead, index size,
and actual query plans. The `LOWER(name)` index supports the exact
normalized-name lookup shown in the SQL query; it does not make a broad
wildcard search efficient.

### 6.3 Author filter query

``` sql
SELECT
    a.id,
    a.author_id,
    a.title,
    a.body,
    a.created_at,
    a.updated_at
FROM articles AS a
JOIN authors AS au ON au.id = a.author_id
WHERE LOWER(BTRIM(au.name)) = LOWER(BTRIM($1))
ORDER BY a.created_at DESC, a.id DESC
LIMIT $2;
```

If using this expression, create a matching expression index:

``` sql
CREATE INDEX idx_authors_normalized_name
    ON authors (LOWER(BTRIM(name)));
```

Choose one consistent normalization expression for both index and query.
Do not create both author-name indexes without a measured reason.

### 6.4 Keyset pagination

For a subsequent page, use the last returned `(created_at, id)` pair:

``` sql
SELECT id, author_id, title, body, created_at, updated_at
FROM articles
WHERE (created_at, id) < ($1, $2)
ORDER BY created_at DESC, id DESC
LIMIT $3;
```

For author filtering, add the author-ID/name predicate while retaining
the same cursor condition.

Cursor payload should be opaque to clients and validated when decoded.
It should include the sort values and, where necessary, a version and
filter fingerprint. Sign the cursor if tamper resistance is required.

Keyset pagination avoids deep OFFSET scans. It does not create a fully
frozen snapshot across separate requests while new rows are inserted;
document this behavior.

### 6.5 Transaction requirements

Create and update operations must insert the outbox event in the same
PostgreSQL transaction as the article mutation.

If either insert/update or outbox insertion fails, roll back the
transaction.

Use `RETURNING` to obtain persisted values. Never update `created_at` in
the update statement.

### 6.6 Data volume and partitioning

The target workload is approximately 50 million articles with growth of
200,000 per day. The design must be benchmarked with representative
data.

Do not partition solely because the row count is large. First measure: -
Query plans and latency - Table and index size - Vacuum and maintenance
behavior - Backup/restore impact - Retention and operational
requirements

If partitioning is justified, time-based partitioning is a candidate.
Validate how primary/unique key constraints and queries behave with the
chosen partition key before adopting it.

------------------------------------------------------------------------

## 7. Elasticsearch design

### 7.1 Index document

Use one document per article, with deterministic document ID equal to
the PostgreSQL article ID.

Proposed fields:

  Field                      Elasticsearch type   Purpose
  -------------------------- -------------------- --------------------------
  `article_id`               `long`               Article identity
  `author_id`                `long`               Author identity
  `author_name_normalized`   `keyword`            Exact author-name filter
  `title`                    `text`               Title full-text search
  `body`                     `text`               Body full-text search
  `created_at`               `date`               Sort
  `updated_at`               `date`               Freshness/debugging

### 7.2 Initial mapping

Use the standard analyzer as a neutral baseline for mixed-language
content. It tokenizes text but does not provide language-specific
stemming or semantic understanding.

``` json
{
  "settings": {
    "number_of_shards": 3,
    "number_of_replicas": 0,
    "analysis": {
      "normalizer": {
        "lowercase_normalizer": {
          "type": "custom",
          "filter": ["lowercase", "asciifolding"]
        }
      }
    }
  },
  "mappings": {
    "properties": {
      "article_id": { "type": "long" },
      "author_id": { "type": "long" },
      "author_name_normalized": {
        "type": "keyword",
        "normalizer": "lowercase_normalizer"
      },
      "title": { "type": "text", "analyzer": "standard" },
      "body": { "type": "text", "analyzer": "standard" },
      "created_at": { "type": "date" },
      "updated_at": { "type": "date" }
    }
  }
}
```

Shard count is a local starting configuration, not a production
recommendation. Production shard sizing must be based on indexed data
size, node count, query load, and benchmark results. Production replicas
should be configured for availability.

### 7.3 Search query

Use `multi_match` across title and body and apply the author-name filter
when provided.

``` json
{
  "query": {
    "bool": {
      "must": [
        {
          "multi_match": {
            "query": "golang backend",
            "fields": ["title^2", "body"],
            "type": "best_fields"
          }
        }
      ],
      "filter": [
        {
          "term": {
            "author_name_normalized": "budi"
          }
        }
      ]
    }
  },
  "sort": [
    { "created_at": "desc" },
    { "article_id": "desc" }
  ]
}
```

Omit the author filter when no author parameter is supplied. Omit the
keyword `must` clause when no keyword is supplied; the no-keyword
listing path should use PostgreSQL rather than an unbounded
Elasticsearch query.

### 7.4 Mixed-language strategy

Initial approach: - Use the standard analyzer for title and body. - Test
Indonesian, English, and mixed-language documents. - Evaluate stemming
and relevance using a labeled test corpus. - Only add language-specific
multi-fields or custom analyzers if measured search quality requires
them.

Do not assign one language to an entire article. A single article may
contain both languages. Language-specific analysis can be applied to
multi-fields, but it increases index size and query complexity.

### 7.5 Pagination

Use `search_after` with a Point-in-Time (PIT) for deep and stable search
pagination. Sort values must include a deterministic tie-breaker such as
`article_id`.

Do not use deep `from`/`size` pagination for large result offsets.

### 7.6 Index lifecycle and recovery

-   Create a versioned index, e.g. `articles-v1`.
-   Use an alias, e.g. `articles-read`, for application queries.
-   Use deterministic document IDs.
-   Use bulk indexing for initial backfill and reindexing.
-   Record failed indexing attempts.
-   Support rebuilding the index from PostgreSQL.
-   Switch aliases only after the replacement index has been validated.

------------------------------------------------------------------------

## 8. Search consistency and outbox processing

### 8.1 Consistency model

PostgreSQL commits are synchronous and authoritative. Elasticsearch is
eventually consistent.

A successful POST/PATCH means the PostgreSQL transaction committed. It
does not necessarily mean the search index has already been updated.

Document this behavior in API documentation.

### 8.2 Event payload

Example:

``` json
{
  "event_id": 123,
  "event_type": "article.updated",
  "article_id": 101,
  "author_id": 1,
  "title": "Updated title",
  "body": "Updated body",
  "created_at": "2026-11-02T10:00:00Z",
  "updated_at": "2026-11-02T11:00:00Z"
}
```

The payload should contain enough information for the worker to index
the current article. Alternatively, the worker can load the latest
article row by ID. Choose and document one approach.

### 8.3 Worker lifecycle

1.  Poll pending events.
2.  Claim a bounded batch safely using a transaction and row locking
    (`FOR UPDATE SKIP LOCKED`).
3.  Process each event.
4.  Index the article using its deterministic document ID.
5.  Mark the event processed only after successful indexing.
6.  On failure, increment attempts and set a retry time.
7.  Log the error and retain the event for retry.
8.  Move repeatedly failing events to a visible dead-letter/error state
    or alert for operator action.

Use bounded concurrency and batch size. Do not hold a database
transaction open while waiting on a slow Elasticsearch request.
Implement a claim/lease strategy so abandoned work can be reclaimed
after worker crashes.

### 8.4 Ordering and stale-event protection

Multiple updates to the same article can be processed out of order.
Prevent an older event from overwriting a newer indexed version.

Recommended approach: - Include a monotonic version in the article or
event model, incremented transactionally on every update. - Use
Elasticsearch external versioning or an equivalent conditional update
strategy. - Ensure retries are idempotent. - Test create/update races
and worker restart scenarios.

If adding a version column, include it in the schema and explain it as
an implementation requirement for safe event ordering.

### 8.5 Search freshness and stale data

The search path returns candidate IDs from Elasticsearch and loads
current article rows from PostgreSQL. This prevents returning old
title/body values for documents that still exist, but it cannot discover
an article that has not yet been indexed.

The API must document eventual search visibility. If strict
read-after-write search visibility is required, add an explicit
mechanism (for example, a bounded wait or a direct fallback for the
newly written article); do not imply that the outbox alone guarantees
immediate visibility.

When PostgreSQL no longer returns a candidate ID, skip it and record a
metric. If filtering or missing candidates reduce a page, the
application may need to fetch additional search hits to fill the page
while preserving order.

------------------------------------------------------------------------

## 9. Redis caching design

### 9.1 Cache scope

Cache GET responses for: - Popular unfiltered listing pages - Repeated
keyword searches - Repeated author searches

Avoid caching highly unique requests if hit rate is low.

### 9.2 Cache key

Build a deterministic key from: - API/cache schema version - Normalized
keyword - Normalized author name - Page size - Cursor or search-after
token

Example:

``` text
articles:v1:q=golang:author=budi:limit=20:cursor=first
```

Hash long or sensitive query components rather than storing them
directly in keys.

### 9.3 TTL and invalidation

Use a configurable short TTL as the initial policy (for example, 30--60
seconds), then tune from observed traffic and freshness requirements.

On article create/update: - Invalidate affected cached listing/search
entries, or - Prefer short TTL plus versioned cache namespaces if broad
invalidation is too expensive.

Do not attempt to enumerate every possible search query key. A practical
strategy is to use short TTLs and a namespace/version key that can be
advanced after writes, while considering the cache churn this causes.

### 9.4 Cache failure

-   Redis timeout/unavailability must fall back to the underlying read
    path.
-   Cache errors should be logged and measured.
-   Never return fabricated data when cache parsing fails.
-   Prevent cache stampedes for hot keys using request
    coalescing/singleflight or a short-lived lock where appropriate.

------------------------------------------------------------------------

## 10. API contract

Base path: `/`

Content type: `application/json`

### 10.1 POST `/articles`

Request:

``` json
{
  "author_id": 1,
  "title": "Introduction to Golang",
  "body": "Golang is a programming language..."
}
```

Success: `201 Created`

``` json
{
  "data": {
    "id": 101,
    "author_id": 1,
    "title": "Introduction to Golang",
    "body": "Golang is a programming language...",
    "created_at": "2026-11-02T10:00:00Z",
    "updated_at": "2026-11-02T10:00:00Z"
  }
}
```

Validation: - `author_id` must be positive. - `title` and `body` must
not be blank. - Apply a documented maximum request body size. - Author
must exist.

### 10.2 GET `/articles`

Parameters:

  Parameter   Type            Default   Rule
  ----------- --------------- --------- ------------------------------------
  `query`     string          absent    Keyword search in title/body
  `author`    string          absent    Exact normalized author-name match
  `limit`     integer         20        Range 1--100
  `cursor`    opaque string   absent    Validated keyset/search cursor

Examples:

``` http
GET /articles
GET /articles?query=golang
GET /articles?author=Budi
GET /articles?query=golang&author=Budi
GET /articles?limit=20&cursor=<opaque-token>
```

Success: `200 OK`

``` json
{
  "data": [],
  "pagination": {
    "limit": 20,
    "next_cursor": null,
    "has_more": false
  }
}
```

The cursor format is opaque and may differ between PostgreSQL listing
and Elasticsearch search. Do not promise that a cursor from one query
can be reused with different filters.

### 10.3 PATCH `/articles/{id}`

Request:

``` json
{
  "title": "Updated title",
  "body": "Updated body"
}
```

Rules: - At least one supported field must be supplied. - Distinguish
omitted fields from explicitly empty values. - Empty title/body is
invalid. - `created_at` never changes. - `updated_at` changes only when
an update succeeds.

Success: `200 OK`.

### 10.4 Operational endpoints

-   `GET /health`: process liveness.
-   `GET /ready`: readiness of required dependencies.

Do not expose credentials or detailed internal infrastructure errors.

### 10.5 Error response

``` json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Title is required"
  }
}
```

Use consistent error codes and HTTP status mapping. Log internal causes
separately.

------------------------------------------------------------------------

## 11. Go implementation conventions

### 11.1 Context and timeouts

-   Pass `context.Context` through handlers, services, repositories, and
    clients.
-   Set explicit request and dependency timeouts.
-   Cancel downstream work when the client disconnects where
    appropriate.
-   Use graceful shutdown.

### 11.2 SQL

-   Use positional parameters.
-   Never concatenate user input into SQL.
-   Use transactions for article mutation plus outbox insertion.
-   Select only required columns.
-   Batch-load article IDs rather than issuing one query per result.
-   Preserve Elasticsearch result ordering after PostgreSQL batch
    retrieval.

### 11.3 Error handling

Define domain/application errors such as: - `ErrArticleNotFound` -
`ErrAuthorNotFound` - `ErrInvalidInput` - `ErrInvalidCursor`

Map these to HTTP responses in the handler layer. Do not return raw SQL
or Elasticsearch errors to clients.

### 11.4 Logging

Use structured logs with: - Request ID - Route and method - Status
code - Duration - Error category - Worker event ID and article ID where
applicable

Do not log full article bodies or secrets by default.

### 11.5 Configuration

Use environment variables and validate required configuration at
startup.

Example `.env.example`:

``` dotenv
APP_ENV=development
HTTP_ADDR=:8080
HTTP_READ_TIMEOUT=5s
HTTP_WRITE_TIMEOUT=10s
HTTP_IDLE_TIMEOUT=60s

POSTGRES_DSN=postgres://app:app@postgres:5432/articles?sslmode=disable
POSTGRES_MAX_CONNS=20
POSTGRES_MIN_CONNS=2

ELASTICSEARCH_URL=http://elasticsearch:9200
ELASTICSEARCH_INDEX=articles-v1

REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_DB=0
CACHE_TTL=45s

OUTBOX_BATCH_SIZE=100
OUTBOX_POLL_INTERVAL=1s
OUTBOX_MAX_ATTEMPTS=10
```

Values are local development examples, not production sizing
recommendations. Never commit real secrets.

------------------------------------------------------------------------

## 12. Testing strategy

### 12.1 Unit tests

Test: - Request validation - Author normalization - Cursor
encoding/decoding - Business rules - Update semantics - Error mapping -
Cache-key generation

### 12.2 PostgreSQL integration tests

Test: - Article insert - Missing author - Duplicate author names -
Newest-first ordering - Keyset pagination - Update while preserving
`created_at` - Transaction rollback when outbox insert fails - SQL query
behavior with skewed author data

### 12.3 Elasticsearch integration tests

Test: - Title keyword search - Body keyword search - Indonesian
content - English content - Mixed-language content - Author filter -
Combined keyword and author filter - Stable sort and `search_after` -
Reindex and retry behavior

Use a small deterministic relevance corpus. Assert expected documents
rather than relying only on scores, unless score ranking itself is under
test.

### 12.4 Redis integration tests

Test: - Cache hit - Cache miss - Expiration - Cache
invalidation/versioning - Redis unavailable fallback - Concurrent
requests for the same cache key

### 12.5 Outbox tests

Test: - Event created atomically with article - Successful processing -
Retry after transient error - Worker crash and lease recovery -
Duplicate event delivery - Out-of-order updates - Dead-letter/alert
behavior

### 12.6 Performance tests

Use k6 scenarios: 1. PostgreSQL listing without filters 2. Author
filtering for long-tail authors 3. Author filtering for the 30%
syndication author 4. Elasticsearch keyword search 5. Combined keyword
and author search 6. Redis cache hit 7. Redis cache miss 8. Bursty
concurrent reads 9. Mixed read/write workload at approximately 100:1

Record p50, p95, p99, throughput, error rate, database pool usage, cache
hit ratio, and search latency.

Do not claim that the system supports 50 million rows until a
representative benchmark has been run. Use synthetic data with the
specified author skew.

------------------------------------------------------------------------

## 13. Observability and operations

Minimum operational signals:

  Area               Metric/log
  ------------------ ------------------------------------------------------
  HTTP               Request count, latency, status/error rate
  PostgreSQL         Pool usage, query latency, connection errors
  Elasticsearch      Search latency, indexing failures, rejected requests
  Redis              Hit/miss ratio, latency, errors
  Outbox             Pending count, oldest event age, retry count
  Worker             Processing rate, failures, lease recovery
  Search freshness   Delay between DB commit and successful indexing

Operational requirements: - Health and readiness checks - Graceful
shutdown - Bounded worker concurrency - Retry with backoff and jitter -
Database backup/restore plan documented - Elasticsearch rebuild
procedure documented - Logs must not expose secrets or article contents
unnecessarily

------------------------------------------------------------------------

## 14. Docker Compose design

Local services: - `api` - `worker` - `postgres` - `redis` -
`elasticsearch`

Requirements: - Persistent volumes for stateful services - Health checks
and startup dependency handling - Internal network for
databases/search/cache - Only API port exposed to host by default -
Resource limits where supported - Pinned image versions -
Environment-based configuration

The local Elasticsearch configuration may use a single node and reduced
replica count for development only. This is not a production
high-availability topology.

------------------------------------------------------------------------

## 15. Architecture Decision Records

Create an ADR for each significant choice.

### ADR-001: REST API and Gin

**Decision:** Use REST with Gin.\
**Reason:** The required use cases are simple resource-oriented HTTP
operations.\
**Trade-off:** No schema-driven GraphQL selection or gRPC contract.

### ADR-002: PostgreSQL as source of truth

**Decision:** Store authoritative article and author data in
PostgreSQL.\
**Reason:** Transactions, foreign keys, and explicit SQL support the
required data integrity.\
**Trade-off:** Search workloads are delegated to Elasticsearch.

### ADR-003: Elasticsearch for full-text search

**Decision:** Use Elasticsearch for keyword search over title and body.\
**Reason:** Dedicated full-text indexing is more appropriate than
leading-wildcard `LIKE` scans over tens of millions of rows.\
**Trade-off:** Additional infrastructure and eventual consistency.

### ADR-004: Transactional Outbox

**Decision:** Write article changes and outbox events in one PostgreSQL
transaction.\
**Reason:** Avoid losing index-update events when the database commits
but the search service is unavailable.\
**Trade-off:** Worker, retry, ordering, and monitoring complexity.

### ADR-005: Redis as optional cache

**Decision:** Cache selected read responses with bounded TTL.\
**Reason:** Reduce repeated work under read-heavy traffic.\
**Trade-off:** Invalidation and stale-cache behavior must be managed.

### ADR-006: Cursor-based pagination

**Decision:** Use keyset pagination for PostgreSQL listing and
`search_after`/PIT for Elasticsearch search.\
**Reason:** Avoid expensive deep OFFSET pagination.\
**Trade-off:** Cursors are opaque and tied to sort/filter context.

### ADR-007: Duplicate author names

**Decision:** `author` filters by exact normalized name and includes
every matching author ID.\
**Reason:** The assessment explicitly permits duplicate names and asks
for a stated resolution.\
**Trade-off:** A common name may match many authors and articles.

### ADR-008: Mixed-language analysis

**Decision:** Begin with standard analyzer and validate with a
mixed-language corpus.\
**Reason:** Language is not recorded and one article may mix languages.\
**Trade-off:** Standard analyzer does not provide language-specific
stemming; relevance may need later tuning.

------------------------------------------------------------------------

## 16. Implementation sequence

### Phase 1 --- Foundation

-   Initialize Go module and project structure
-   Add configuration and logging
-   Add PostgreSQL connection pool
-   Add Goose migrations
-   Create authors, articles, and outbox tables
-   Add required indexes
-   Add health/readiness endpoints

### Phase 2 --- Core API

-   Implement domain and application service
-   Implement PostgreSQL repositories
-   Implement `POST /articles`
-   Implement `GET /articles` without keyword
-   Implement author filter
-   Implement cursor pagination
-   Add unit and PostgreSQL integration tests

### Phase 3 --- Search

-   Add Elasticsearch client and index mapping
-   Implement indexing repository
-   Implement keyword search
-   Implement author-name filtering in the search index
-   Implement combined filters
-   Implement search pagination
-   Add mixed-language tests

### Phase 4 --- Consistency and update

-   Implement transactional outbox
-   Implement worker claiming, retry, and recovery
-   Implement idempotent indexing and event ordering
-   Implement `PATCH /articles/{id}`
-   Validate `created_at` immutability

### Phase 5 --- Cache and operations

-   Add Redis client and cache policy
-   Add cache invalidation/versioning
-   Add cache stampede protection
-   Add structured metrics/logging
-   Add Docker Compose

### Phase 6 --- Validation and submission

-   Run all unit and integration tests
-   Run k6 scenarios
-   Generate representative skewed dataset
-   Inspect PostgreSQL `EXPLAIN (ANALYZE, BUFFERS)`
-   Review Elasticsearch query behavior
-   Document limitations and trade-offs
-   Complete README and AI usage disclosure

------------------------------------------------------------------------

## 17. Definition of Done

A feature is complete only when: 1. Its requirement and acceptance
criteria are documented. 2. Its implementation follows the approved
design. 3. SQL is parameterized and no ORM is used. 4. Error cases are
handled. 5. Relevant automated tests pass. 6. Code has been reviewed. 7.
Actual behavior has been validated. 8. Documentation is updated.

A successful compile is not sufficient evidence of correctness.

------------------------------------------------------------------------

## 18. Open questions and items requiring validation

The following items must be resolved or explicitly documented before
production claims are made:

1.  Exact maximum title/body/request sizes.
2.  Expected API freshness requirement after create/update.
3.  Whether author matching should be case-insensitive exact matching
    (proposed) or case-sensitive exact matching.
4.  Expected search semantics: all terms, any terms, phrase search, and
    ranking expectations.
5.  Target infrastructure size and production availability requirements.
6.  Elasticsearch shard/replica sizing based on measured index size.
7.  Whether partitioning is justified by benchmark and operational
    requirements.
8.  Cursor behavior when new articles are inserted between page
    requests.
9.  Cache freshness and invalidation policy.
10. Data migration/backfill procedure for the initial 50-million-article
    corpus.

Do not silently invent answers to these open questions. Record the
selected behavior in an ADR or implementation specification.

------------------------------------------------------------------------

## 19. AI-assisted development rules

AI tools may assist with: - Exploring alternatives - Drafting code -
Reviewing SQL and architecture - Generating test cases - Writing
documentation

The developer remains responsible for: - Approving requirements and
design decisions - Reviewing every generated change - Running tests and
inspecting actual results - Rejecting unsupported or incorrect
suggestions - Recording a real example of an AI failure and the
corrective action

Every implementation task should provide the AI with: 1. Relevant
specification 2. Existing project context 3. Explicit constraints 4.
Expected files and interfaces 5. Acceptance criteria 6. Required tests

AI must not independently change the architecture or add scope without
approval.

------------------------------------------------------------------------

**End of Technical Design Document**
