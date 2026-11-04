package elasticsearch

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/kumparan/article-service/internal/config"
	"github.com/kumparan/article-service/internal/domain/article"
)

func TestNew_RequiresURLAndIndex(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.ESConfig
	}{
		{name: "missing URL", cfg: config.ESConfig{Index: "articles-v1"}},
		{name: "missing index", cfg: config.ESConfig{URL: "http://localhost:9200"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.cfg); err == nil {
				t.Fatal("New returned nil error")
			}
		})
	}
}

func TestSearchBuildsMultiMatchAndParsesIDs(t *testing.T) {
	var requestBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestBody, _ = io.ReadAll(r.Body)
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits":{"hits":[{"_id":"42"},{"_id":"7"}]}}`))
	}))
	defer server.Close()

	client, err := New(config.ESConfig{URL: server.URL, Index: "articles-v1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ids, err := client.Search(context.Background(), "programming", "budi", 20)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 2 || ids[0] != 42 || ids[1] != 7 {
		t.Fatalf("ids = %v, want [42 7]", ids)
	}
	if !bytes.Contains(requestBody, []byte(`"bool"`)) ||
		!bytes.Contains(requestBody, []byte(`"multi_match"`)) ||
		!bytes.Contains(requestBody, []byte(`"title^2"`)) ||
		!bytes.Contains(requestBody, []byte(`"best_fields"`)) {
		t.Fatalf("request body does not contain expected multi_match: %s", requestBody)
	}
}

func TestSearchBuildsAuthorFilter(t *testing.T) {
	var requestBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestBody, _ = io.ReadAll(r.Body)
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		_, _ = w.Write([]byte(`{"hits":{"hits":[]}}`))
	}))
	defer server.Close()
	client, err := New(config.ESConfig{URL: server.URL, Index: "articles-v1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Search(context.Background(), "golang", "budi", 20); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(requestBody, []byte(`"filter"`)) ||
		!bytes.Contains(requestBody, []byte(`"author_name_normalized"`)) ||
		!bytes.Contains(requestBody, []byte(`"budi"`)) {
		t.Fatalf("request body does not contain author term filter: %s", requestBody)
	}
}

func TestSearch_Integration_FindsTitleAndBodyTerms(t *testing.T) {
	url := os.Getenv("ELASTICSEARCH_URL")
	if url == "" {
		t.Skip("ELASTICSEARCH_URL not set; skipping Elasticsearch integration test")
	}

	client, err := New(config.ESConfig{URL: url, Index: "articles-v1-impl13-test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	deleteIndex(t, client, client.Index())
	t.Cleanup(func() { deleteIndex(t, client, client.Index()) })
	if err := client.EnsureIndex(context.Background()); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	author := &article.Author{ID: 1, Name: "Search Author"}
	for _, item := range []*article.Article{
		{ID: 9101, AuthorID: 1, Title: "pemrograman bahasa", Body: "konten"},
		{ID: 9102, AuthorID: 1, Title: "programming guide", Body: "content"},
		{ID: 9103, AuthorID: 1, Title: "mixed language", Body: "pemrograman programming"},
	} {
		if err := client.IndexArticle(context.Background(), item, author); err != nil {
			t.Fatalf("IndexArticle(%d): %v", item.ID, err)
		}
	}
	refresh, err := client.Raw().Indices.Refresh(
		client.Raw().Indices.Refresh.WithIndex(client.Index()),
	)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	refresh.Body.Close()
	for _, query := range []string{"pemrograman", "programming"} {
		ids, err := client.Search(context.Background(), query, "", 20)
		if err != nil {
			t.Fatalf("Search(%q): %v", query, err)
		}
		if len(ids) == 0 {
			t.Fatalf("Search(%q) returned no results", query)
		}
	}
}

func TestEnsureIndex_Integration_CreatesMappingAndIsIdempotent(t *testing.T) {
	url := os.Getenv("ELASTICSEARCH_URL")
	if url == "" {
		t.Skip("ELASTICSEARCH_URL not set; skipping Elasticsearch integration test")
	}

	index := "articles-v1-impl11-test"
	client, err := New(config.ESConfig{URL: url, Index: index})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	deleteIndex(t, client, index)
	t.Cleanup(func() { deleteIndex(t, client, index) })

	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if err := client.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex first call: %v", err)
	}
	if err := client.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex second call: %v", err)
	}

	mapping, err := client.Raw().Indices.GetMapping(
		client.Raw().Indices.GetMapping.WithContext(ctx),
		client.Raw().Indices.GetMapping.WithIndex(index),
	)
	if err != nil {
		t.Fatalf("get mapping: %v", err)
	}
	defer mapping.Body.Close()
	if mapping.IsError() {
		t.Fatalf("get mapping: %s", mapping.String())
	}
	mappingBody := mapping.String()
	for _, field := range []string{
		`"article_id"`,
		`"author_id"`,
		`"author_name_normalized"`,
		`"title"`,
		`"body"`,
		`"created_at"`,
		`"updated_at"`,
	} {
		if !strings.Contains(mappingBody, field) {
			t.Errorf("mapping does not contain %s: %s", field, mappingBody)
		}
	}
	if strings.Contains(mappingBody, "asciifolding") {
		t.Error("author normalizer must not contain asciifolding")
	}

	aliases, err := client.Raw().Indices.GetAlias(
		client.Raw().Indices.GetAlias.WithContext(ctx),
		client.Raw().Indices.GetAlias.WithName(client.Alias()),
	)
	if err != nil {
		t.Fatalf("get alias: %v", err)
	}
	defer aliases.Body.Close()
	if aliases.IsError() {
		t.Fatalf("get alias: %s", aliases.String())
	}
	if !strings.Contains(aliases.String(), `"`+index+`"`) {
		t.Fatalf("alias %q does not point to %q: %s", client.Alias(), index, aliases.String())
	}
}

func TestSearchPage_Integration_PITSearchAfter(t *testing.T) {
	url := os.Getenv("ELASTICSEARCH_URL")
	if url == "" {
		t.Skip("ELASTICSEARCH_URL not set; skipping Elasticsearch integration test")
	}
	client, err := New(config.ESConfig{URL: url, Index: "articles-v1-impl15-test"})
	if err != nil {
		t.Fatal(err)
	}
	deleteIndex(t, client, client.Index())
	t.Cleanup(func() { deleteIndex(t, client, client.Index()) })
	if err := client.EnsureIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	author := &article.Author{ID: 1, Name: "PIT Author"}
	for id := int64(9201); id <= 9225; id++ {
		if err := client.IndexArticle(context.Background(), &article.Article{
			ID: id, AuthorID: 1, Title: "pagination keyword", Body: "stable result",
		}, author); err != nil {
			t.Fatal(err)
		}
	}
	refresh, err := client.Raw().Indices.Refresh(client.Raw().Indices.Refresh.WithIndex(client.Index()))
	if err != nil {
		t.Fatal(err)
	}
	refresh.Body.Close()
	first, err := client.SearchPage(context.Background(), "pagination", "pit author", "", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasMore || len(first.IDs) != 10 || first.PITID == "" {
		t.Fatalf("first page = %#v", first)
	}
	second, err := client.SearchPage(context.Background(), "pagination", "pit author", first.PITID, first.LastSort, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !second.HasMore || len(second.IDs) != 10 {
		t.Fatalf("second page = %#v", second)
	}
	third, err := client.SearchPage(context.Background(), "pagination", "pit author", second.PITID, second.LastSort, 10)
	if err != nil {
		t.Fatal(err)
	}
	if third.HasMore || len(third.IDs) != 5 {
		t.Fatalf("third page = %#v", third)
	}
	if err := client.ClosePIT(context.Background(), third.PITID); err != nil {
		t.Fatal(err)
	}
}

func deleteIndex(t *testing.T, client *Client, index string) {
	t.Helper()
	res, err := client.Raw().Indices.Delete(
		[]string{index},
		client.Raw().Indices.Delete.WithIgnoreUnavailable(true),
	)
	if err != nil {
		t.Fatalf("delete index: %v", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		t.Fatalf("delete index: %s", res.String())
	}
}
