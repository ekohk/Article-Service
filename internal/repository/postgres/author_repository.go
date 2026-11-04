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

func (r *AuthorRepository) Create(ctx context.Context, name string) (*article.Author, error) {
	const q = `
		INSERT INTO authors (name)
		VALUES ($1)
		RETURNING id, name, created_at`
	var a article.Author
	if err := r.db.QueryRowContext(ctx, q, name).Scan(&a.ID, &a.Name, &a.CreatedAt); err != nil {
		return nil, fmt.Errorf("author Create: %w", err)
	}
	return &a, nil
}

func (r *AuthorRepository) List(ctx context.Context) ([]*article.Author, error) {
	const q = `
		SELECT id, name, created_at
		FROM authors
		ORDER BY id ASC`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("author List: %w", err)
	}
	defer rows.Close()

	authors := make([]*article.Author, 0)
	for rows.Next() {
		author := new(article.Author)
		if err := rows.Scan(&author.ID, &author.Name, &author.CreatedAt); err != nil {
			return nil, fmt.Errorf("author List scan: %w", err)
		}
		authors = append(authors, author)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("author List rows: %w", err)
	}
	return authors, nil
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
