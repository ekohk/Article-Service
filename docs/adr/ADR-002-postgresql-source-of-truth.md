# ADR-002: PostgreSQL as Source of Truth

- **Status:** Accepted
- **Decision:** PostgreSQL is authoritative for authors, articles, and outbox
  events.
- **Context:** Article mutations require relational integrity and atomic
  article/outbox transactions.
- **Consequences:** PostgreSQL owns correctness and Elasticsearch is a derived
  projection. Search requires hydration from PostgreSQL.
