package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kumparan/article-service/internal/domain/article"
)

// AuthorRepository is the PostgreSQL implementation of article.AuthorRepository.
type AuthorRepository struct {
	db *sql.DB
}

// NewAuthorRepository creates an AuthorRepository backed by db.
func NewAuthorRepository(db *sql.DB) *AuthorRepository {
	return &AuthorRepository{db: db}
}

// GetByID returns the author with the given id or article.ErrAuthorNotFound.
func (r *AuthorRepository) GetByID(ctx context.Context, id int64) (*article.Author, error) {
	const q = `
		SELECT id, name, created_at
		FROM authors
		WHERE id = $1`

	row := r.db.QueryRowContext(ctx, q, id)
	var a article.Author
	if err := row.Scan(&a.ID, &a.Name, &a.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, article.ErrAuthorNotFound
		}
		return nil, fmt.Errorf("author GetByID: %w", err)
	}
	return &a, nil
}

// GetIDsByNormalizedName returns all author IDs whose LOWER(BTRIM(name))
// matches the provided normalizedName (D-01). Returns an empty slice when none match.
func (r *AuthorRepository) GetIDsByNormalizedName(ctx context.Context, normalizedName string) ([]int64, error) {
	const q = `
		SELECT id
		FROM authors
		WHERE LOWER(BTRIM(name)) = $1`

	rows, err := r.db.QueryContext(ctx, q, normalizedName)
	if err != nil {
		return nil, fmt.Errorf("author GetIDsByNormalizedName: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("author GetIDsByNormalizedName scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
