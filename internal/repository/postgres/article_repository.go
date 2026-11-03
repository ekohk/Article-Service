// Package postgres provides SQL-backed implementations of the domain repository
// interfaces. All queries use parameterised SQL; no ORM is used.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kumparan/article-service/internal/domain/article"
)

// ArticleRepository is the PostgreSQL implementation of article.ArticleRepository.
type ArticleRepository struct {
	db *sql.DB
}

// NewArticleRepository creates an ArticleRepository backed by db.
func NewArticleRepository(db *sql.DB) *ArticleRepository {
	return &ArticleRepository{db: db}
}

// Insert persists a new article inside the provided transaction and returns the
// row as committed by the database (id, created_at, updated_at from RETURNING).
func (r *ArticleRepository) Insert(ctx context.Context, tx *sql.Tx, a *article.Article) (*article.Article, error) {
	const q = `
		INSERT INTO articles (author_id, title, body, version)
		VALUES ($1, $2, $3, 1)
		RETURNING id, author_id, title, body, version, created_at, updated_at`

	row := tx.QueryRowContext(ctx, q, a.AuthorID, a.Title, a.Body)
	return scanArticle(row)
}

// Update modifies title and/or body, increments version, and refreshes updated_at.
// created_at is never touched. Returns the updated row.
func (r *ArticleRepository) Update(ctx context.Context, tx *sql.Tx, a *article.Article) (*article.Article, error) {
	const q = `
		UPDATE articles
		SET title      = $1,
		    body       = $2,
		    version    = version + 1,
		    updated_at = NOW()
		WHERE id = $3
		RETURNING id, author_id, title, body, version, created_at, updated_at`

	row := tx.QueryRowContext(ctx, q, a.Title, a.Body, a.ID)
	updated, err := scanArticle(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, article.ErrNotFound
		}
		return nil, err
	}
	return updated, nil
}

// GetByID returns the article with the given id or article.ErrNotFound.
func (r *ArticleRepository) GetByID(ctx context.Context, id int64) (*article.Article, error) {
	const q = `
		SELECT id, author_id, title, body, version, created_at, updated_at
		FROM articles
		WHERE id = $1`

	row := r.db.QueryRowContext(ctx, q, id)
	a, err := scanArticle(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, article.ErrNotFound
		}
		return nil, fmt.Errorf("article GetByID: %w", err)
	}
	return a, nil
}

// GetByIDs fetches multiple articles in one query. Order is undefined.
// IDs not found are silently omitted (used to hydrate Elasticsearch results).
func (r *ArticleRepository) GetByIDs(ctx context.Context, ids []int64) ([]*article.Article, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// Build a parameterised IN clause: ($1,$2,...)
	placeholders := make([]byte, 0, len(ids)*4)
	args := make([]any, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, []byte(fmt.Sprintf("$%d", i+1))...)
		args[i] = id
	}

	q := fmt.Sprintf(`
		SELECT id, author_id, title, body, version, created_at, updated_at
		FROM articles
		WHERE id IN (%s)`, string(placeholders))

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("article GetByIDs: %w", err)
	}
	defer rows.Close()

	var articles []*article.Article
	for rows.Next() {
		a, err := scanArticleRow(rows)
		if err != nil {
			return nil, fmt.Errorf("article GetByIDs scan: %w", err)
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

// List returns articles ordered by (created_at DESC, id DESC) with optional
// author-name filter and keyset pagination. Implements IMPL-08 contract.
func (r *ArticleRepository) List(ctx context.Context, p article.ListParams) ([]*article.Article, error) {
	// Built in IMPL-08; placeholder to satisfy interface in IMPL-07.
	return nil, fmt.Errorf("List not yet implemented")
}

// ── scan helpers ──────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

func scanArticle(row *sql.Row) (*article.Article, error) {
	var a article.Article
	err := row.Scan(
		&a.ID, &a.AuthorID, &a.Title, &a.Body,
		&a.Version, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func scanArticleRow(rows *sql.Rows) (*article.Article, error) {
	var a article.Article
	err := rows.Scan(
		&a.ID, &a.AuthorID, &a.Title, &a.Body,
		&a.Version, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}
