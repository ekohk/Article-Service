# ADR-006: Cursor-Based Pagination

- **Status:** Accepted
- **Decision:** Use PostgreSQL keyset pagination for listing and Elasticsearch
  `search_after` with PIT for search.
- **Context:** Deep offset pagination becomes expensive at large scale.
- **Consequences:** Cursors are opaque and bound to sort/filter context.
  PostgreSQL listing is not a frozen snapshot; Elasticsearch cursors have a
  PIT lifecycle and expiration behavior.
