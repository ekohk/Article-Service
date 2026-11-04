// Package elasticsearch provides the Elasticsearch client and article index
// bootstrap used by the worker and search repositories.
package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	es "github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"

	"github.com/kumparan/article-service/internal/config"
	"github.com/kumparan/article-service/internal/domain/article"
)

const (
	defaultAlias = "articles-read"
)

// Client wraps the Elasticsearch client and article index configuration.
type Client struct {
	client *es.Client
	index  string
	alias  string
}

type searchRequest struct {
	Query struct {
		Bool struct {
			Must   []any `json:"must"`
			Filter []any `json:"filter,omitempty"`
		} `json:"bool"`
	} `json:"query"`
	Sort []any `json:"sort"`
	Size int   `json:"size"`
	PIT  *struct {
		ID        string `json:"id"`
		KeepAlive string `json:"keep_alive"`
	} `json:"pit,omitempty"`
	SearchAfter []json.RawMessage `json:"search_after,omitempty"`
}

type searchResponse struct {
	Hits struct {
		Hits []struct {
			ID   string            `json:"_id"`
			Sort []json.RawMessage `json:"sort"`
		} `json:"hits"`
	} `json:"hits"`
	PITID string `json:"pit_id"`
}

// New creates an Elasticsearch client from the validated application config.
func New(cfg config.ESConfig) (*Client, error) {
	if cfg.URL == "" {
		return nil, errors.New("elasticsearch: URL is required")
	}
	if cfg.Index == "" {
		return nil, errors.New("elasticsearch: index is required")
	}

	client, err := es.NewClient(es.Config{
		Addresses: []string{cfg.URL},
		Username:  cfg.Username,
		Password:  cfg.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: create client: %w", err)
	}

	return &Client{
		client: client,
		index:  cfg.Index,
		alias:  defaultAlias,
	}, nil
}

// Raw returns the underlying official Elasticsearch client for repositories
// that need operations beyond index bootstrap.
func (c *Client) Raw() *es.Client {
	return c.client
}

// Index returns the configured article index name.
func (c *Client) Index() string {
	return c.index
}

// Alias returns the read alias used for article searches.
func (c *Client) Alias() string {
	return c.alias
}

// Search returns article IDs in deterministic relevance order.
func (c *Client) Search(ctx context.Context, query, authorName string, limit int) ([]int64, error) {
	if limit < 1 {
		return nil, errors.New("elasticsearch: search limit must be positive")
	}
	body := c.buildSearchRequest(query, authorName, limit, "", nil)
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	res, err := c.client.Search(c.client.Search.WithContext(ctx), c.client.Search.WithIndex(c.alias), c.client.Search.WithBody(bytes.NewReader(payload)))
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: search: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch: search: %s", responseError(res))
	}
	var decoded searchResponse
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("elasticsearch: decode search response: %w", err)
	}
	return parseSearchIDs(decoded)
}

func (c *Client) buildSearchRequest(query, authorName string, size int, pitID string, after []json.RawMessage) searchRequest {
	var body searchRequest
	body.Query.Bool.Must = []any{map[string]any{"multi_match": map[string]any{
		"query": query, "fields": []string{"title^2", "body"}, "type": "best_fields",
	}}}
	if authorName != "" {
		body.Query.Bool.Filter = []any{map[string]any{"term": map[string]string{"author_name_normalized": authorName}}}
	}
	body.Sort = []any{map[string]string{"_score": "desc"}, map[string]string{"article_id": "desc"}}
	body.Size = size
	body.SearchAfter = after
	if pitID != "" {
		body.PIT = &struct {
			ID        string `json:"id"`
			KeepAlive string `json:"keep_alive"`
		}{ID: pitID, KeepAlive: pitKeepAlive}
	}
	return body
}

func parseSearchIDs(decoded searchResponse) ([]int64, error) {
	ids := make([]int64, 0, len(decoded.Hits.Hits))
	for _, hit := range decoded.Hits.Hits {
		id, err := strconv.ParseInt(hit.ID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("elasticsearch: invalid article document ID %q: %w", hit.ID, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

const pitKeepAlive = "2m"

func (c *Client) SearchPage(ctx context.Context, query, authorName, pitID string, searchAfter []json.RawMessage, limit int) (article.SearchPage, error) {
	if limit < 1 {
		return article.SearchPage{}, errors.New("elasticsearch: search limit must be positive")
	}
	openedHere := pitID == ""
	if openedHere {
		res, err := c.client.OpenPointInTime([]string{c.alias}, pitKeepAlive,
			c.client.OpenPointInTime.WithContext(ctx))
		if err != nil {
			return article.SearchPage{}, fmt.Errorf("elasticsearch: open PIT: %w", err)
		}
		defer res.Body.Close()
		if res.IsError() {
			return article.SearchPage{}, fmt.Errorf("elasticsearch: open PIT: %s", responseError(res))
		}
		var opened struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(res.Body).Decode(&opened); err != nil {
			return article.SearchPage{}, fmt.Errorf("elasticsearch: decode PIT response: %w", err)
		}
		if opened.ID == "" {
			return article.SearchPage{}, errors.New("elasticsearch: PIT response did not contain an ID")
		}
		pitID = opened.ID
	}
	body := c.buildSearchRequest(query, authorName, limit+1, pitID, searchAfter)

	payload, err := json.Marshal(body)
	if err != nil {
		return article.SearchPage{}, fmt.Errorf("elasticsearch: encode search: %w", err)
	}
	res, err := c.client.Search(
		c.client.Search.WithContext(ctx),
		c.client.Search.WithBody(bytes.NewReader(payload)),
	)
	if err != nil {
		if openedHere {
			_ = c.ClosePIT(ctx, pitID)
		}
		return article.SearchPage{}, fmt.Errorf("elasticsearch: search: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		if openedHere {
			_ = c.ClosePIT(ctx, pitID)
		}
		message := readResponseBody(res.Body)
		if isMissingPITResponse(res.StatusCode, message) {
			return article.SearchPage{}, fmt.Errorf("%w: %s", article.ErrPITExpired, message)
		}
		return article.SearchPage{}, fmt.Errorf("elasticsearch: search: status %d: %s", res.StatusCode, message)
	}

	var decoded searchResponse
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		if openedHere {
			_ = c.ClosePIT(ctx, pitID)
		}
		return article.SearchPage{}, fmt.Errorf("elasticsearch: decode search response: %w", err)
	}
	hasMore := len(decoded.Hits.Hits) > limit
	hits := decoded.Hits.Hits
	if hasMore {
		hits = hits[:limit]
	}
	ids := make([]int64, 0, len(hits))
	var lastSort []json.RawMessage
	for _, hit := range hits {
		id, err := strconv.ParseInt(hit.ID, 10, 64)
		if err != nil {
			if openedHere {
				_ = c.ClosePIT(ctx, pitID)
			}
			return article.SearchPage{}, fmt.Errorf("elasticsearch: invalid article document ID %q: %w", hit.ID, err)
		}
		ids = append(ids, id)
		lastSort = hit.Sort
	}
	if decoded.PITID == "" {
		decoded.PITID = pitID
	}
	return article.SearchPage{IDs: ids, PITID: decoded.PITID, LastSort: lastSort, HasMore: hasMore}, nil
}

func isMissingPITResponse(status int, message string) bool {
	if status != http.StatusNotFound {
		return false
	}
	message = strings.ToLower(message)
	return strings.Contains(message, "search_context_missing_exception") ||
		strings.Contains(message, "point_in_time_missing_exception") ||
		strings.Contains(message, "no search context found") ||
		strings.Contains(message, "no point in time found")
}

func (c *Client) ClosePIT(ctx context.Context, pitID string) error {
	if pitID == "" {
		return nil
	}
	body, err := json.Marshal(map[string]string{"id": pitID})
	if err != nil {
		return fmt.Errorf("elasticsearch: encode close PIT: %w", err)
	}
	res, err := c.client.ClosePointInTime(
		c.client.ClosePointInTime.WithContext(ctx),
		c.client.ClosePointInTime.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return fmt.Errorf("elasticsearch: close PIT: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("elasticsearch: close PIT: %s", responseError(res))
	}
	return nil
}

// IndexArticle writes the latest article projection using its database version.
func (c *Client) IndexArticle(ctx context.Context, a *article.Article, author *article.Author) error {
	body, err := json.Marshal(struct {
		ArticleID            int64     `json:"article_id"`
		AuthorID             int64     `json:"author_id"`
		AuthorNameNormalized string    `json:"author_name_normalized"`
		Title                string    `json:"title"`
		Body                 string    `json:"body"`
		CreatedAt            time.Time `json:"created_at"`
		UpdatedAt            time.Time `json:"updated_at"`
		Version              int64     `json:"version"`
	}{
		ArticleID:            a.ID,
		AuthorID:             a.AuthorID,
		AuthorNameNormalized: article.NormalizeAuthorName(author.Name),
		Title:                a.Title,
		Body:                 a.Body,
		CreatedAt:            a.CreatedAt,
		UpdatedAt:            a.UpdatedAt,
		Version:              a.Version,
	})
	if err != nil {
		return fmt.Errorf("elasticsearch: encode article: %w", err)
	}

	res, err := c.client.Index(
		c.index,
		bytes.NewReader(body),
		c.client.Index.WithContext(ctx),
		c.client.Index.WithDocumentID(strconv.FormatInt(a.ID, 10)),
		c.client.Index.WithVersion(int(a.Version)),
		c.client.Index.WithVersionType("external_gte"),
	)
	if err != nil {
		return fmt.Errorf("elasticsearch: index article: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() && res.StatusCode != http.StatusConflict {
		return &IndexError{StatusCode: res.StatusCode, Message: readResponseBody(res.Body)}
	}
	return nil
}

type IndexError struct {
	StatusCode int
	Message    string
}

func (e *IndexError) Error() string {
	return fmt.Sprintf("elasticsearch index: status %d: %s", e.StatusCode, e.Message)
}

func (e *IndexError) Retryable() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

// Ping verifies that Elasticsearch is reachable.
func (c *Client) Ping(ctx context.Context) error {
	res, err := c.client.Ping(c.client.Ping.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("elasticsearch: ping: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("elasticsearch: ping: %s", responseError(res))
	}
	return nil
}

// EnsureIndex creates the article index and read alias when needed.
// Existing indexes are left unchanged so repeated startup is idempotent.
func (c *Client) EnsureIndex(ctx context.Context) error {
	exists, err := c.client.Indices.Exists(
		[]string{c.index},
		c.client.Indices.Exists.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("elasticsearch: check index: %w", err)
	}
	switch exists.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		if err := c.createIndex(ctx); err != nil {
			return err
		}
	default:
		defer exists.Body.Close()
		return fmt.Errorf("elasticsearch: check index: %s", responseError(exists))
	}
	exists.Body.Close()

	alias, err := c.client.Indices.PutAlias(
		[]string{c.index},
		c.alias,
		c.client.Indices.PutAlias.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("elasticsearch: create alias: %w", err)
	}
	defer alias.Body.Close()
	if alias.IsError() {
		return fmt.Errorf("elasticsearch: create alias: %s", responseError(alias))
	}
	return nil
}

func (c *Client) createIndex(ctx context.Context) error {
	res, err := c.client.Indices.Create(
		c.index,
		c.client.Indices.Create.WithContext(ctx),
		c.client.Indices.Create.WithBody(bytes.NewReader([]byte(indexDefinition))),
	)
	if err != nil {
		return fmt.Errorf("elasticsearch: create index: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("elasticsearch: create index: %s", responseError(res))
	}
	return nil
}

func responseError(res *esapi.Response) string {
	return fmt.Sprintf("status %d: %s", res.StatusCode, readResponseBody(res.Body))
}

const indexDefinition = `{
  "settings": {
    "number_of_shards": 3,
    "number_of_replicas": 0,
    "analysis": {
      "normalizer": {
        "lowercase_normalizer": {
          "type": "custom",
          "filter": ["lowercase"]
        }
      }
    }
  },
  "mappings": {
    "properties": {
      "article_id": { "type": "long" },
      "author_id": { "type": "long" },
      "author_name_normalized": {
        "type": "keyword",
        "normalizer": "lowercase_normalizer"
      },
      "title": { "type": "text", "analyzer": "standard" },
      "body": { "type": "text", "analyzer": "standard" },
      "created_at": { "type": "date" },
      "updated_at": { "type": "date" },
      "version": { "type": "long" }
    }
  }
}`

func readResponseBody(body io.Reader) string {
	data, err := io.ReadAll(body)
	if err != nil {
		return "unable to read response"
	}
	var payload struct {
		Error struct {
			Reason string `json:"reason"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &payload) == nil && payload.Error.Reason != "" {
		return payload.Error.Reason
	}
	return string(data)
}
