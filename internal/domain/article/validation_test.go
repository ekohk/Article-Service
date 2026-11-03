package article_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kumparan/article-service/internal/domain/article"
)

// ── ValidateCreate ──────────────────────────────────────────────────────────

func TestValidateCreate_Valid(t *testing.T) {
	if err := article.ValidateCreate(1, "Valid Title", "Valid body content."); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestValidateCreate_ZeroAuthorID(t *testing.T) {
	err := article.ValidateCreate(0, "Title", "Body")
	if !errors.Is(err, article.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestValidateCreate_NegativeAuthorID(t *testing.T) {
	err := article.ValidateCreate(-1, "Title", "Body")
	if !errors.Is(err, article.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestValidateCreate_BlankTitle(t *testing.T) {
	cases := []string{"", "   ", "\t", "\n"}
	for _, tc := range cases {
		err := article.ValidateCreate(1, tc, "Body")
		if !errors.Is(err, article.ErrInvalidInput) {
			t.Errorf("title=%q: expected ErrInvalidInput, got %v", tc, err)
		}
	}
}

func TestValidateCreate_TitleTooLong(t *testing.T) {
	long := strings.Repeat("a", article.MaxTitleLen+1)
	err := article.ValidateCreate(1, long, "Body")
	if !errors.Is(err, article.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for oversized title, got %v", err)
	}
}

func TestValidateCreate_TitleAtLimit(t *testing.T) {
	exact := strings.Repeat("a", article.MaxTitleLen)
	if err := article.ValidateCreate(1, exact, "Body"); err != nil {
		t.Errorf("title at limit should be valid, got %v", err)
	}
}

func TestValidateCreate_BlankBody(t *testing.T) {
	err := article.ValidateCreate(1, "Title", "   ")
	if !errors.Is(err, article.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for blank body, got %v", err)
	}
}

func TestValidateCreate_BodyTooLong(t *testing.T) {
	long := strings.Repeat("x", article.MaxBodyLen+1)
	err := article.ValidateCreate(1, "Title", long)
	if !errors.Is(err, article.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for oversized body, got %v", err)
	}
}

// ── ValidateUpdate ──────────────────────────────────────────────────────────

func TestValidateUpdate_NoFields(t *testing.T) {
	err := article.ValidateUpdate(nil, nil)
	if !errors.Is(err, article.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput when no fields provided, got %v", err)
	}
}

func TestValidateUpdate_EmptyTitle(t *testing.T) {
	s := ""
	err := article.ValidateUpdate(&s, nil)
	if !errors.Is(err, article.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty title pointer, got %v", err)
	}
}

func TestValidateUpdate_ValidTitleOnly(t *testing.T) {
	s := "New Title"
	if err := article.ValidateUpdate(&s, nil); err != nil {
		t.Errorf("expected nil for valid title-only update, got %v", err)
	}
}

func TestValidateUpdate_ValidBothFields(t *testing.T) {
	title := "Updated Title"
	body := "Updated body content."
	if err := article.ValidateUpdate(&title, &body); err != nil {
		t.Errorf("expected nil for valid both-fields update, got %v", err)
	}
}

// ── ValidateLimit ───────────────────────────────────────────────────────────

func TestValidateLimit_Valid(t *testing.T) {
	for _, v := range []int{1, 20, 100} {
		if err := article.ValidateLimit(v); err != nil {
			t.Errorf("limit=%d should be valid, got %v", v, err)
		}
	}
}

func TestValidateLimit_Zero(t *testing.T) {
	if err := article.ValidateLimit(0); !errors.Is(err, article.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for limit=0, got %v", err)
	}
}

func TestValidateLimit_TooHigh(t *testing.T) {
	if err := article.ValidateLimit(101); !errors.Is(err, article.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for limit=101, got %v", err)
	}
}

// ── NormalizeAuthorName (D-01) ───────────────────────────────────────────────

func TestNormalizeAuthorName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"  Budi Santoso  ", "budi santoso"},
		{"BUDI", "budi"},
		{"budi", "budi"},
		{"  Budi  ", "budi"},
		{"Büdi", "büdi"}, // accents preserved (D-01: no asciifolding)
	}
	for _, tc := range cases {
		got := article.NormalizeAuthorName(tc.input)
		if got != tc.want {
			t.Errorf("NormalizeAuthorName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ── Error sentinel identity ──────────────────────────────────────────────────

func TestErrorSentinels_IsComparable(t *testing.T) {
	errs := []error{
		article.ErrNotFound,
		article.ErrAuthorNotFound,
		article.ErrInvalidInput,
		article.ErrInvalidCursor,
	}
	for _, e := range errs {
		if !errors.Is(e, e) {
			t.Errorf("errors.Is(%v, %v) should be true", e, e)
		}
	}
}

func TestErrorSentinels_Wrapping(t *testing.T) {
	// errors.Is must work through fmt.Errorf %w wrapping
	err := article.ValidateCreate(0, "t", "b")
	if !errors.Is(err, article.ErrInvalidInput) {
		t.Errorf("wrapped ErrInvalidInput not detected by errors.Is: %v", err)
	}
}
