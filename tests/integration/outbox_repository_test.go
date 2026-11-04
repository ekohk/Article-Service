package integration_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	domain "github.com/kumparan/article-service/internal/domain/article"
	pgrepo "github.com/kumparan/article-service/internal/repository/postgres"
)

const (
	testLeaseTTL    = time.Minute
	testMaxAttempts = 2
)

func isolateOutbox(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `DELETE FROM outbox_events`); err != nil {
		t.Fatalf("clear outbox: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM outbox_events`)
	})
}

func insertOutboxEvent(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(context.Background(), `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload)
		VALUES ('article', 1, 'article.created', '{}'::jsonb)
		RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatalf("insert outbox event: %v", err)
	}
	return id
}

func newOutboxRepo(db *sql.DB) *pgrepo.OutboxRepository {
	return pgrepo.NewOutboxRepository(db, testLeaseTTL, testMaxAttempts)
}

func TestOutbox_Integration_ConcurrentClaimsDoNotOverlap(t *testing.T) {
	db := setupDB(t)
	isolateOutbox(t, db)
	for range 10 {
		insertOutboxEvent(t, db)
	}

	type claimResult struct {
		events []*domain.OutboxEvent
		err    error
	}
	start := make(chan struct{})
	results := make(chan claimResult, 2)
	var wg sync.WaitGroup
	for _, workerID := range []string{"worker-a", "worker-b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			<-start
			events, err := newOutboxRepo(db).ClaimBatch(context.Background(), 10, id)
			results <- claimResult{events: events, err: err}
		}(workerID)
	}
	close(start)
	wg.Wait()
	close(results)

	seen := map[int64]struct{}{}
	for result := range results {
		if result.err != nil {
			t.Fatalf("ClaimBatch: %v", result.err)
		}
		for _, event := range result.events {
			if _, duplicate := seen[event.ID]; duplicate {
				t.Errorf("event %d was claimed by both workers", event.ID)
			}
			seen[event.ID] = struct{}{}
		}
	}
	if len(seen) != 10 {
		t.Errorf("claimed %d unique events, want 10", len(seen))
	}
}

func TestOutbox_Integration_MarkFailedRecordsRetryAndClearsLease(t *testing.T) {
	db := setupDB(t)
	isolateOutbox(t, db)
	id := insertOutboxEvent(t, db)
	repo := newOutboxRepo(db)

	claimed, err := repo.ClaimBatch(context.Background(), 1, "worker-a")
	if err != nil || len(claimed) != 1 || claimed[0].ID != id {
		t.Fatalf("claim = %+v, %v; want event %d", claimed, err, id)
	}
	next := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	if err := repo.MarkFailed(context.Background(), id, "temporary index failure", next); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	var attempts int
	var lastError string
	var availableAt time.Time
	var lockedAt sql.NullTime
	var lockedBy sql.NullString
	var isDead bool
	err = db.QueryRowContext(context.Background(), `
		SELECT attempts, last_error, available_at, locked_at, locked_by, is_dead
		FROM outbox_events WHERE id = $1`, id).
		Scan(&attempts, &lastError, &availableAt, &lockedAt, &lockedBy, &isDead)
	if err != nil {
		t.Fatalf("read failed event: %v", err)
	}
	if attempts != 1 || lastError != "temporary index failure" || isDead {
		t.Errorf("event state attempts=%d last_error=%q is_dead=%v", attempts, lastError, isDead)
	}
	if !availableAt.Equal(next) {
		t.Errorf("available_at = %s, want %s", availableAt, next)
	}
	if lockedAt.Valid || lockedBy.Valid {
		t.Errorf("lease was not cleared: locked_at=%v locked_by=%v", lockedAt, lockedBy)
	}
}

func TestOutbox_Integration_ActiveLeaseIsNotStolenAndExpiredLeaseIsReclaimed(t *testing.T) {
	db := setupDB(t)
	isolateOutbox(t, db)
	id := insertOutboxEvent(t, db)
	repo := newOutboxRepo(db)

	first, err := repo.ClaimBatch(context.Background(), 1, "worker-a")
	if err != nil || len(first) != 1 || first[0].ID != id {
		t.Fatalf("first claim = %+v, %v", first, err)
	}
	second, err := repo.ClaimBatch(context.Background(), 1, "worker-b")
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("active lease was stolen: %+v", second)
	}

	if _, err := db.ExecContext(context.Background(), `
		UPDATE outbox_events
		SET locked_at = NOW() - INTERVAL '2 minutes', locked_by = 'crashed-worker'
		WHERE id = $1`, id); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	reclaimed, err := repo.ClaimBatch(context.Background(), 1, "worker-b")
	if err != nil || len(reclaimed) != 1 || reclaimed[0].ID != id {
		t.Fatalf("reclaim = %+v, %v; want event %d", reclaimed, err, id)
	}
	if reclaimed[0].LockedBy != "worker-b" {
		t.Errorf("locked_by = %q, want worker-b", reclaimed[0].LockedBy)
	}
}

func TestOutbox_Integration_MaxAttemptsMovesEventToDeadLetter(t *testing.T) {
	db := setupDB(t)
	isolateOutbox(t, db)
	id := insertOutboxEvent(t, db)
	repo := newOutboxRepo(db)

	for attempt := 1; attempt <= testMaxAttempts; attempt++ {
		claimed, err := repo.ClaimBatch(context.Background(), 1, fmt.Sprintf("worker-%d", attempt))
		if err != nil || len(claimed) != 1 || claimed[0].ID != id {
			t.Fatalf("claim attempt %d = %+v, %v", attempt, claimed, err)
		}
		if err := repo.MarkFailed(context.Background(), id, fmt.Sprintf("failure-%d", attempt), time.Now().UTC().Add(-time.Second)); err != nil {
			t.Fatalf("MarkFailed attempt %d: %v", attempt, err)
		}
	}

	var attempts int
	var isDead bool
	if err := db.QueryRowContext(context.Background(), `SELECT attempts, is_dead FROM outbox_events WHERE id = $1`, id).Scan(&attempts, &isDead); err != nil {
		t.Fatalf("read dead-letter event: %v", err)
	}
	if attempts != testMaxAttempts || !isDead {
		t.Errorf("event state attempts=%d is_dead=%v, want %d and true", attempts, isDead, testMaxAttempts)
	}
	claimed, err := repo.ClaimBatch(context.Background(), 1, "worker-after-dead-letter")
	if err != nil {
		t.Fatalf("ClaimBatch after dead letter: %v", err)
	}
	if len(claimed) != 0 {
		t.Errorf("dead-letter event was claimed again: %+v", claimed)
	}
}

func TestOutbox_Integration_MarkProcessedIsTerminalAndClearsLease(t *testing.T) {
	db := setupDB(t)
	isolateOutbox(t, db)
	id := insertOutboxEvent(t, db)
	repo := newOutboxRepo(db)

	if claimed, err := repo.ClaimBatch(context.Background(), 1, "worker-a"); err != nil || len(claimed) != 1 {
		t.Fatalf("claim = %+v, %v", claimed, err)
	}
	if err := repo.MarkProcessed(context.Background(), id); err != nil {
		t.Fatalf("MarkProcessed: %v", err)
	}

	var firstProcessedAt time.Time
	var lockedAt sql.NullTime
	var lockedBy sql.NullString
	if err := db.QueryRowContext(context.Background(), `
		SELECT processed_at, locked_at, locked_by FROM outbox_events WHERE id = $1`, id).
		Scan(&firstProcessedAt, &lockedAt, &lockedBy); err != nil {
		t.Fatalf("read processed event: %v", err)
	}
	if lockedAt.Valid || lockedBy.Valid {
		t.Errorf("lease was not cleared: locked_at=%v locked_by=%v", lockedAt, lockedBy)
	}
	if err := repo.MarkProcessed(context.Background(), id); err != nil {
		t.Fatalf("second MarkProcessed: %v", err)
	}
	var secondProcessedAt time.Time
	if err := db.QueryRowContext(context.Background(), `SELECT processed_at FROM outbox_events WHERE id = $1`, id).Scan(&secondProcessedAt); err != nil {
		t.Fatalf("read processed event after second call: %v", err)
	}
	if !secondProcessedAt.Equal(firstProcessedAt) {
		t.Errorf("processed_at changed from %s to %s", firstProcessedAt, secondProcessedAt)
	}
	claimed, err := repo.ClaimBatch(context.Background(), 1, "worker-b")
	if err != nil {
		t.Fatalf("claim after processing: %v", err)
	}
	if len(claimed) != 0 {
		t.Errorf("processed event was claimed again: %+v", claimed)
	}
}
