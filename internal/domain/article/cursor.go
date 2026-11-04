package article

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// cursorKindListing identifies PostgreSQL newest-first listing cursors.
// Search cursors (IMPL-15) will use a different kind so they cannot be mixed.
const cursorKindListing = "pg_list"

type cursorPayload struct {
	Kind       string `json:"k"`
	CreatedAt  int64  `json:"t"`
	ID         int64  `json:"i"`
	AuthorName string `json:"a,omitempty"`
}

// EncodeCursor returns an opaque URL-safe token for a listing keyset position.
func EncodeCursor(c Cursor) (string, error) {
	raw, err := json.Marshal(cursorPayload{
		Kind:       cursorKindListing,
		CreatedAt:  c.CreatedAt,
		ID:         c.ID,
		AuthorName: c.AuthorName,
	})
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// DecodeCursor parses an opaque listing cursor.
// An empty string means the first page and returns (nil, nil).
func DecodeCursor(s string) (*Cursor, error) {
	if s == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed cursor", ErrInvalidCursor)
	}
	var p cursorPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("%w: malformed cursor", ErrInvalidCursor)
	}
	if p.Kind != cursorKindListing || p.CreatedAt <= 0 || p.ID <= 0 {
		return nil, fmt.Errorf("%w: malformed cursor", ErrInvalidCursor)
	}
	return &Cursor{CreatedAt: p.CreatedAt, ID: p.ID, AuthorName: p.AuthorName}, nil
}
