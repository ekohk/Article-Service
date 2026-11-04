# ADR-007: Duplicate Author Names

- **Status:** Accepted
- **Decision:** An author filter is an exact normalized name match and includes
  every author ID with that normalized name.
- **Context:** Duplicate author names are allowed by the domain and required
  to be handled explicitly.
- **Consequences:** A common name can match many authors and articles. The
  normalization is `LOWER(BTRIM(name))` without ASCII folding.
