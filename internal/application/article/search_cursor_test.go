package article

import (
	"encoding/json"
	"errors"
	"testing"

	domain "github.com/kumparan/article-service/internal/domain/article"
)

func TestSearchCursorRoundTripAndFingerprintValidation(t *testing.T) {
	after := []json.RawMessage{json.RawMessage(`1.5`), json.RawMessage(`42`)}
	token, err := encodeSearchCursor("pit-1", after, "golang", "budi")
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := decodeSearchCursor(token, "golang", "budi")
	if err != nil || cursor.PITID != "pit-1" || len(cursor.After) != 2 {
		t.Fatalf("cursor = %#v, err = %v", cursor, err)
	}
	if _, err := decodeSearchCursor(token, "other", "budi"); !errors.Is(err, domain.ErrInvalidCursor) {
		t.Fatalf("mismatched query error = %v", err)
	}
}

func TestSearchCursorRejectsMalformedInput(t *testing.T) {
	if _, err := decodeSearchCursor("not-a-cursor", "q", ""); !errors.Is(err, domain.ErrInvalidCursor) {
		t.Fatalf("error = %v", err)
	}
}
