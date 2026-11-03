package article

import (
	"fmt"
	"strings"
)

// Limits defined per D-03 (Approved).
const (
	MaxTitleLen  = 500
	MaxBodyLen   = 50_000
	MinLimit     = 1
	MaxLimit     = 100
	DefaultLimit = 20
)

// ValidateCreate checks the invariants required to create an article.
// Returns a wrapped ErrInvalidInput on the first violation found.
func ValidateCreate(authorID int64, title, body string) error {
	if authorID <= 0 {
		return fmt.Errorf("%w: author_id must be positive", ErrInvalidInput)
	}
	if err := validateTitle(title); err != nil {
		return err
	}
	return validateBody(body)
}

// ValidateUpdate checks that at least one field is provided and all provided
// fields are non-blank and within size limits.
func ValidateUpdate(title, body *string) error {
	if title == nil && body == nil {
		return fmt.Errorf("%w: at least one of title or body must be provided", ErrInvalidInput)
	}
	if title != nil {
		if err := validateTitle(*title); err != nil {
			return err
		}
	}
	if body != nil {
		if err := validateBody(*body); err != nil {
			return err
		}
	}
	return nil
}

// ValidateLimit checks that the requested page size is within the allowed range.
func ValidateLimit(limit int) error {
	if limit < MinLimit || limit > MaxLimit {
		return fmt.Errorf("%w: limit must be between %d and %d", ErrInvalidInput, MinLimit, MaxLimit)
	}
	return nil
}

// NormalizeAuthorName applies D-01: LOWER(BTRIM) without asciifolding.
// Accented characters are treated as distinct (e.g. "Büdi" ≠ "Budi").
func NormalizeAuthorName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func validateTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("%w: title must not be blank", ErrInvalidInput)
	}
	if len(title) > MaxTitleLen {
		return fmt.Errorf("%w: title exceeds maximum length of %d characters", ErrInvalidInput, MaxTitleLen)
	}
	return nil
}

func validateBody(body string) error {
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("%w: body must not be blank", ErrInvalidInput)
	}
	if len(body) > MaxBodyLen {
		return fmt.Errorf("%w: body exceeds maximum length of %d characters", ErrInvalidInput, MaxBodyLen)
	}
	return nil
}
