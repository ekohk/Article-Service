# ADR-003: Elasticsearch for Full-Text Search

- **Status:** Accepted
- **Decision:** Use Elasticsearch for keyword search over article title and
  body.
- **Context:** Full-text search over a large article corpus should not depend
  on leading-wildcard PostgreSQL scans.
- **Consequences:** Search has an additional dependency and eventual
  consistency. Results are hydrated from PostgreSQL while preserving
  Elasticsearch order.
