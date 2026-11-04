package article

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	domain "github.com/kumparan/article-service/internal/domain/article"
)

const searchCursorKind = "es_search"

type searchCursor struct {
	Kind        string            `json:"k"`
	PITID       string            `json:"p"`
	After       []json.RawMessage `json:"s"`
	Fingerprint string            `json:"f"`
}

func searchFingerprint(query, author string) string {
	sum := sha256.Sum256([]byte(query + "\x00" + author))
	return hex.EncodeToString(sum[:])
}

func encodeSearchCursor(pitID string, after []json.RawMessage, query, author string) (string, error) {
	raw, err := json.Marshal(searchCursor{Kind: searchCursorKind, PITID: pitID, After: after, Fingerprint: searchFingerprint(query, author)})
	if err != nil {
		return "", fmt.Errorf("encode search cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeSearchCursor(token, query, author string) (*searchCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed search cursor", domain.ErrInvalidCursor)
	}
	var cursor searchCursor
	if err := json.Unmarshal(raw, &cursor); err != nil ||
		cursor.Kind != searchCursorKind || cursor.PITID == "" || len(cursor.After) == 0 ||
		cursor.Fingerprint != searchFingerprint(query, author) {
		return nil, fmt.Errorf("%w: mismatched search cursor", domain.ErrInvalidCursor)
	}
	return &cursor, nil
}
