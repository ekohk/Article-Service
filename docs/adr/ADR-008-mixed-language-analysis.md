# ADR-008: Mixed-Language Analysis

- **Status:** Accepted
- **Decision:** Start Elasticsearch indexing with the standard analyzer for
  mixed Indonesian/English content.
- **Context:** Language metadata is not present and one article can contain
  multiple languages.
- **Consequences:** The baseline is language-neutral and simple, but does not
  provide language-specific stemming. Relevance can be tuned later using a
  representative corpus.
