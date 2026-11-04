# ADR-004: Transactional Outbox

- **Status:** Accepted
- **Decision:** Write article mutations and minimal outbox events in one
  PostgreSQL transaction; a worker indexes the latest article state.
- **Context:** A database commit must not lose the corresponding search-index
  update.
- **Consequences:** The worker needs leases, retries, backoff, ordering,
  dead-letter handling, and operational logging.
