-- +goose Up
-- +goose StatementBegin

-- Dead-letter state is explicit so normal workers never reclaim exhausted events.
ALTER TABLE outbox_events
    ADD COLUMN is_dead BOOLEAN NOT NULL DEFAULT FALSE;

-- Replace the pending index so events in the dead-letter state do not grow the
-- worker's claimable-event index.
DROP INDEX IF EXISTS idx_outbox_pending;
CREATE INDEX idx_outbox_claimable
    ON outbox_events (available_at, id)
    WHERE processed_at IS NULL AND is_dead = FALSE;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_outbox_claimable;
ALTER TABLE outbox_events
    DROP COLUMN IF EXISTS is_dead;
CREATE INDEX idx_outbox_pending
    ON outbox_events (available_at, id)
    WHERE processed_at IS NULL;

-- +goose StatementEnd
