# ADR-005: Redis as Optional Cache

- **Status:** Accepted
- **Decision:** Cache selected GET listing/search responses in Redis with a
  short configurable TTL.
- **Context:** The workload is read-heavy and repeated reads can avoid source
  work.
- **Consequences:** Redis is not a source of truth. Cache timeout, invalid
  payload, or unavailability falls back to the source path; singleflight
  protects hot cache misses.
