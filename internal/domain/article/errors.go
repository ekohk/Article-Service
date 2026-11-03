package article

import "errors"

// Sentinel errors used throughout the application layer and mapped to HTTP
// responses in the handler layer. Never return raw infrastructure errors
// (SQL, Elasticsearch) to callers outside the repository.
var (
	// ErrNotFound is returned when an article does not exist.
	ErrNotFound = errors.New("article not found")

	// ErrAuthorNotFound is returned when the referenced author_id does not exist.
	ErrAuthorNotFound = errors.New("author not found")

	// ErrInvalidInput is returned when request data fails validation rules
	// (blank title, missing required field, limit out of range, etc.).
	ErrInvalidInput = errors.New("invalid input")

	// ErrInvalidCursor is returned when a pagination cursor cannot be decoded
	// or does not match the current query context.
	ErrInvalidCursor = errors.New("invalid cursor")
)
