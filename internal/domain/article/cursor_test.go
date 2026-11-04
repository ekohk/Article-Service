package article_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kumparan/article-service/internal/domain/article"
)

func TestEncodeDecodeCursor_Roundtrip(t *testing.T) {
	original := article.Cursor{CreatedAt: 1_730_000_000_000_000_000, ID: 42, AuthorName: "budi"}
	token, err := article.EncodeCursor(original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if token == "" {
		t.Fatal("encoded cursor must not be empty")
	}
	got, err := article.DecodeCursor(token)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got == nil {
		t.Fatal("decoded cursor is nil")
	}
	if *got != original {
		t.Errorf("decoded %+v, want %+v", *got, original)
	}
}

func TestDecodeCursor_EmptyMeansFirstPage(t *testing.T) {
	got, err := article.DecodeCursor("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("empty cursor should decode to nil, got %+v", got)
	}
}

func TestDecodeCursor_Corrupt(t *testing.T) {
	cases := []string{
		"not-base64!!!",
		"YWJj", // valid base64, not JSON object
		mustEncode(t, article.Cursor{CreatedAt: 0, ID: 1}),
		mustEncode(t, article.Cursor{CreatedAt: 1, ID: 0}),
	}
	for _, tc := range cases {
		_, err := article.DecodeCursor(tc)
		if !errors.Is(err, article.ErrInvalidCursor) {
			t.Errorf("cursor %q: expected ErrInvalidCursor, got %v", tc, err)
		}
	}
}

func TestDecodeCursor_WrongKind(t *testing.T) {
	raw, err := json.Marshal(map[string]any{"k": "es_search", "t": int64(10), "i": int64(1)})
	if err != nil {
		t.Fatal(err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	_, err = article.DecodeCursor(token)
	if !errors.Is(err, article.ErrInvalidCursor) {
		t.Errorf("expected ErrInvalidCursor for search cursor, got %v", err)
	}
}

func mustEncode(t *testing.T, c article.Cursor) string {
	t.Helper()
	token, err := article.EncodeCursor(c)
	if err != nil {
		t.Fatal(err)
	}
	return token
}
