package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kumparan/article-service/internal/domain/article"
)

// OutboxRepository is the PostgreSQL implementation of article.OutboxRepository.
type OutboxRepository struct {
	db          *sql.DB
	leaseTTL    time.Duration // how long a claim lock is valid before expiry
	maxAttempts int
}

// NewOutboxRepository creates an OutboxRepository.
// leaseTTL is the maximum duration a claimed event can be held before it becomes
// eligible for re-claiming by another worker (handles worker crash recovery).
func NewOutboxRepository(db *sql.DB, leaseTTL time.Duration, maxAttempts int) *OutboxRepository {
	return &OutboxRepository{db: db, leaseTTL: leaseTTL, maxAttempts: maxAttempts}
}

// Insert writes a new pending outbox event within the given transaction.
// The event is inserted atomically with the article mutation.
func (r *OutboxRepository) Insert(ctx context.Context, tx *sql.Tx, e *article.OutboxEvent) error {
	const q = `
		INSERT INTO outbox_events
			(aggregate_type, aggregate_id, event_type, payload)
		VALUES ($1, $2, $3, $4)`

	_, err := tx.ExecContext(ctx, q,
		e.AggregateType, e.AggregateID, e.EventType, e.Payload)
	if err != nil {
		return fmt.Errorf("outbox Insert: %w", err)
	}
	return nil
}

// ClaimBatch atomically locks up to batchSize pending events for workerID.
// It also reclaims events whose lease has expired (worker crash recovery).
func (r *OutboxRepository) ClaimBatch(ctx context.Context, batchSize int, workerID string) ([]*article.OutboxEvent, error) {
	// CTE selects pending events (unprocessed, available now, not held by
	// a live lease) and locks them with FOR UPDATE SKIP LOCKED.
	// A lease is expired when locked_at + leaseTTL < NOW().
	const q = `
		WITH candidates AS (
			SELECT id
			FROM outbox_events
			WHERE processed_at IS NULL
			  AND is_dead = FALSE
			  AND available_at <= NOW()
			  AND (locked_at IS NULL OR locked_at < NOW() - $3::interval)
			ORDER BY available_at, id
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox_events o
		SET locked_by = $2,
		    locked_at = NOW()
		FROM candidates
		WHERE o.id = candidates.id
		RETURNING o.id, o.aggregate_type, o.aggregate_id, o.event_type,
		          o.payload, o.created_at, o.processed_at, o.attempts, o.is_dead,
		          o.available_at, o.locked_at, o.locked_by, o.last_error`

	leaseTTLStr := fmt.Sprintf("%d seconds", int(r.leaseTTL.Seconds()))
	rows, err := r.db.QueryContext(ctx, q, batchSize, workerID, leaseTTLStr)
	if err != nil {
		return nil, fmt.Errorf("outbox ClaimBatch: %w", err)
	}
	defer rows.Close()

	var events []*article.OutboxEvent
	for rows.Next() {
		e, err := scanOutboxEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("outbox ClaimBatch scan: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// MarkProcessed sets processed_at = now() and clears the lease for the event.
func (r *OutboxRepository) MarkProcessed(ctx context.Context, id int64) error {
	const q = `
		UPDATE outbox_events
		SET processed_at = NOW(),
		    locked_by    = NULL,
		    locked_at    = NULL
		WHERE id = $1
		  AND processed_at IS NULL
		  AND is_dead = FALSE`

	_, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("outbox MarkProcessed: %w", err)
	}
	return nil
}

// MarkFailed increments attempts, records the error message, clears the lease,
// and sets the next available_at for retry scheduling.
func (r *OutboxRepository) MarkFailed(ctx context.Context, id int64, errMsg string, nextAvailableAt interface{}) error {
	const q = `
		UPDATE outbox_events
		SET attempts      = attempts + 1,
		    is_dead       = attempts + 1 >= $4,
		    last_error    = $2,
		    available_at  = $3,
		    locked_by     = NULL,
		    locked_at     = NULL
		WHERE id = $1
		  AND processed_at IS NULL
		  AND is_dead = FALSE`

	_, err := r.db.ExecContext(ctx, q, id, errMsg, nextAvailableAt, r.maxAttempts)
	if err != nil {
		return fmt.Errorf("outbox MarkFailed: %w", err)
	}
	return nil
}

// ── scan helper ──────────────────────────────────────────────────────────────

func scanOutboxEvent(rows *sql.Rows) (*article.OutboxEvent, error) {
	var e article.OutboxEvent
	var processedAt sql.NullTime
	var lockedAt sql.NullTime
	var lockedBy sql.NullString
	var lastError sql.NullString

	err := rows.Scan(
		&e.ID, &e.AggregateType, &e.AggregateID, &e.EventType,
		&e.Payload, &e.CreatedAt, &processedAt, &e.Attempts, &e.IsDead,
		&e.AvailableAt, &lockedAt, &lockedBy, &lastError,
	)
	if err != nil {
		return nil, err
	}

	if processedAt.Valid {
		e.ProcessedAt = &processedAt.Time
	}
	if lockedAt.Valid {
		e.LockedAt = &lockedAt.Time
	}
	e.LockedBy = lockedBy.String
	e.LastError = lastError.String

	return &e, nil
}
