# ADR-001: REST API and Gin

- **Status:** Accepted
- **Decision:** Use resource-oriented REST endpoints implemented with Gin.
- **Context:** The service exposes create, list/search, update, and operational
  endpoints with JSON contracts.
- **Consequences:** The API is simple to exercise with HTTP clients and k6.
  GraphQL selection and gRPC contracts are out of scope.
