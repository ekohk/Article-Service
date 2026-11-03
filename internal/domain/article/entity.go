// Package article defines the core domain entities and business invariants
// for the article service.
//
// This package must not import any infrastructure package (postgres, redis,
// elasticsearch). All dependencies flow inward: infrastructure depends on
// domain, never the other way around.
package article

import "time"

// Article is the central aggregate of this service.
type Article struct {
	ID        int64
	AuthorID  int64
	Title     string
	Body      string
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Author represents a content creator. Names are not unique identifiers —
// multiple Author records may share the same Name (see D-01, ADR-007).
type Author struct {
	ID        int64
	Name      string
	CreatedAt time.Time
}

// OutboxEvent records a pending change that must be propagated to the
// search index. It is written in the same transaction as the article mutation.
type OutboxEvent struct {
	ID            int64
	AggregateType string
	AggregateID   int64
	EventType     string
	Payload       []byte
	CreatedAt     time.Time
	ProcessedAt   *time.Time
	Attempts      int
	AvailableAt   time.Time
	LockedAt      *time.Time
	LockedBy      string
	LastError     string
}

// EventType constants used for outbox payloads.
const (
	EventTypeArticleCreated = "article.created"
	EventTypeArticleUpdated = "article.updated"
)
