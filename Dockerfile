# syntax=docker/dockerfile:1

# ── Build stage ────────────────────────────────────────────────────────────────
FROM golang:1.27.1-alpine AS builder
WORKDIR /src

# Cache dependency download separately from source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/api    ./cmd/api/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/worker ./cmd/worker/

# ── API runtime ────────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot AS api
COPY --from=builder /out/api /api
EXPOSE 8080
ENTRYPOINT ["/api"]

# ── Worker runtime ─────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot AS worker
COPY --from=builder /out/worker /worker
ENTRYPOINT ["/worker"]
