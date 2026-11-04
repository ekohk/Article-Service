package article

import (
	"context"
	"database/sql"
)

// ListParams carries validated parameters for listing and filtering articles.
type ListParams struct {
	AuthorName string // normalized: LOWER(BTRIM(name)), empty = no filter
	Limit      int    // validated: 1–100
	Cursor     *Cursor
}

// Cursor encodes the keyset position for stable pagination.
// The cursor is opaque to API consumers; it is base64-encoded JSON internally.
type Cursor struct {
	CreatedAt  int64 // Unix nano
	ID         int64
	AuthorName string // normalized filter context; empty means unfiltered listing
}

// ArticleRepository defines persistence operations for articles.
// Implementations live in internal/repository/postgres.
type ArticleRepository interface {
	// Insert saves a new article and returns the persisted row.
	// Must be called within a transaction provided by the caller.
	Insert(ctx context.Context, tx *sql.Tx, a *Article) (*Article, error)

	// Update modifies title/body, increments version, and updates updated_at.
	// Must not change created_at. Must be called within a caller-provided transaction.
	Update(ctx context.Context, tx *sql.Tx, a *Article) (*Article, error)

	// GetByID returns a single article or ErrNotFound.
	GetByID(ctx context.Context, id int64) (*Article, error)

	// GetByIDs returns articles for the given IDs in any order.
	// Missing IDs are silently omitted (used after ES search).
	GetByIDs(ctx context.Context, ids []int64) ([]*Article, error)

	// List returns articles ordered by (created_at DESC, id DESC) with optional
	// author-name filter and keyset pagination.
	List(ctx context.Context, p ListParams) ([]*Article, error)
}

// AuthorRepository defines read operations for authors.
type AuthorRepository interface {
	Create(ctx context.Context, name string) (*Author, error)
	List(ctx context.Context) ([]*Author, error)

	// GetByID returns the author or ErrAuthorNotFound.
	GetByID(ctx context.Context, id int64) (*Author, error)

	// GetIDsByNormalizedName returns all author IDs whose normalized name matches.
	// Normalization: LOWER(BTRIM(name)). Returns an empty slice (not an error)
	// when no author matches.
	GetIDsByNormalizedName(ctx context.Context, normalizedName string) ([]int64, error)
}

// OutboxRepository defines operations on the transactional outbox table.
type OutboxRepository interface {
	// Insert writes a new pending event within the provided transaction.
	Insert(ctx context.Context, tx *sql.Tx, e *OutboxEvent) error

	// ClaimBatch atomically locks up to batchSize pending events for workerID.
	// Events with expired leases (locked_at older than leaseDuration) are also eligible.
	ClaimBatch(ctx context.Context, batchSize int, workerID string) ([]*OutboxEvent, error)

	// MarkProcessed sets processed_at = now() for the given event ID.
	MarkProcessed(ctx context.Context, id int64) error

	// MarkFailed increments attempts, records the error, clears the lease,
	// and sets available_at = nextAvailableAt for retry scheduling.
	MarkFailed(ctx context.Context, id int64, errMsg string, nextAvailableAt interface{}) error
}
