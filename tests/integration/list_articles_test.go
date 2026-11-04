package integration_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	appsvc "github.com/kumparan/article-service/internal/application/article"
	domain "github.com/kumparan/article-service/internal/domain/article"
)

func isolateArticles(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `DELETE FROM outbox_events`); err != nil {
		t.Fatalf("clear outbox: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM articles`); err != nil {
		t.Fatalf("clear articles: %v", err)
	}
	t.Cleanup(func() {
		db.ExecContext(context.Background(), `DELETE FROM outbox_events`)
		db.ExecContext(context.Background(), `DELETE FROM articles`)
	})
}

func insertArticleAt(t *testing.T, db *sql.DB, authorID int64, title string, createdAt time.Time) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(context.Background(), `
		INSERT INTO articles (author_id, title, body, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)
		RETURNING id`, authorID, title, "body for "+title, createdAt).Scan(&id)
	if err != nil {
		t.Fatalf("insert article %q: %v", title, err)
	}
	return id
}

func assertNewestFirst(t *testing.T, articles []*domain.Article) {
	t.Helper()
	for i := 1; i < len(articles); i++ {
		a, b := articles[i-1], articles[i]
		if a.CreatedAt.Before(b.CreatedAt) {
			t.Errorf("order broken at %d: %v before %v", i, a.CreatedAt, b.CreatedAt)
		}
		if a.CreatedAt.Equal(b.CreatedAt) && a.ID <= b.ID {
			t.Errorf("tie-break broken at %d: id %d then %d", i, a.ID, b.ID)
		}
	}
}

func TestList_Integration_EmptyDataset(t *testing.T) {
	db := setupDB(t)
	isolateArticles(t, db)
	svc := buildSvc(db)

	result, err := svc.List(context.Background(), appsvc.ListRequest{Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Articles) != 0 {
		t.Errorf("len = %d, want 0", len(result.Articles))
	}
	if result.HasMore || result.NextCursor != nil {
		t.Errorf("empty listing must not paginate: has_more=%v cursor=%v", result.HasMore, result.NextCursor)
	}
}

func TestList_Integration_NewestFirst(t *testing.T) {
	db := setupDB(t)
	isolateArticles(t, db)
	authorID := seedAuthor(t, db, "List Author")
	svc := buildSvc(db)

	base := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Hour)
	insertArticleAt(t, db, authorID, "oldest", base)
	insertArticleAt(t, db, authorID, "middle", base.Add(time.Minute))
	insertArticleAt(t, db, authorID, "newest", base.Add(2*time.Minute))
	insertArticleAt(t, db, authorID, "four", base.Add(3*time.Minute))
	insertArticleAt(t, db, authorID, "five", base.Add(4*time.Minute))

	result, err := svc.List(context.Background(), appsvc.ListRequest{Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Articles) != 5 {
		t.Fatalf("len = %d, want 5", len(result.Articles))
	}
	assertNewestFirst(t, result.Articles)
	if result.Articles[0].Title != "five" || result.Articles[4].Title != "oldest" {
		t.Errorf("titles = %s ... %s", result.Articles[0].Title, result.Articles[4].Title)
	}
	if result.HasMore {
		t.Error("has_more should be false for a single page")
	}
}

func TestList_Integration_SameCreatedAtTieBreak(t *testing.T) {
	db := setupDB(t)
	isolateArticles(t, db)
	authorID := seedAuthor(t, db, "Tie Author")
	svc := buildSvc(db)

	ts := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Hour)
	idLow := insertArticleAt(t, db, authorID, "low-id", ts)
	idHigh := insertArticleAt(t, db, authorID, "high-id", ts)
	if idHigh <= idLow {
		t.Fatalf("expected later insert to have higher id, got %d then %d", idLow, idHigh)
	}

	result, err := svc.List(context.Background(), appsvc.ListRequest{Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Articles) != 2 {
		t.Fatalf("len = %d, want 2", len(result.Articles))
	}
	if result.Articles[0].ID != idHigh || result.Articles[1].ID != idLow {
		t.Errorf("got ids %d, %d; want %d then %d", result.Articles[0].ID, result.Articles[1].ID, idHigh, idLow)
	}
}

func TestList_Integration_PaginationNoDuplicates(t *testing.T) {
	db := setupDB(t)
	isolateArticles(t, db)
	authorID := seedAuthor(t, db, "Page Author")
	svc := buildSvc(db)

	base := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Hour)
	var ids []int64
	for i := 0; i < 5; i++ {
		ids = append(ids, insertArticleAt(t, db, authorID, "a"+string(rune('1'+i)), base.Add(time.Duration(i)*time.Minute)))
	}

	page1, err := svc.List(context.Background(), appsvc.ListRequest{Limit: 2})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if !page1.HasMore || page1.NextCursor == nil {
		t.Fatal("page1 should have next_cursor")
	}
	if len(page1.Articles) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1.Articles))
	}

	page2, err := svc.List(context.Background(), appsvc.ListRequest{Limit: 2, Cursor: *page1.NextCursor})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2.Articles) != 2 {
		t.Fatalf("page2 len = %d, want 2", len(page2.Articles))
	}

	page3, err := svc.List(context.Background(), appsvc.ListRequest{Limit: 2, Cursor: *page2.NextCursor})
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	if len(page3.Articles) != 1 {
		t.Fatalf("page3 len = %d, want 1", len(page3.Articles))
	}
	if page3.HasMore || page3.NextCursor != nil {
		t.Error("last page must not have next_cursor")
	}

	seen := map[int64]bool{}
	for _, page := range [][]*domain.Article{page1.Articles, page2.Articles, page3.Articles} {
		for _, a := range page {
			if seen[a.ID] {
				t.Errorf("duplicate article id %d across pages", a.ID)
			}
			seen[a.ID] = true
		}
	}
	if len(seen) != 5 {
		t.Errorf("unique ids = %d, want 5 (also skip check vs %v)", len(seen), ids)
	}
}

func TestList_Integration_NewArticleAfterCursor_D04(t *testing.T) {
	db := setupDB(t)
	isolateArticles(t, db)
	authorID := seedAuthor(t, db, "D04 Author")
	svc := buildSvc(db)

	base := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Hour)
	insertArticleAt(t, db, authorID, "old", base)
	insertArticleAt(t, db, authorID, "mid", base.Add(time.Minute))
	insertArticleAt(t, db, authorID, "new", base.Add(2*time.Minute))

	page1, err := svc.List(context.Background(), appsvc.ListRequest{Limit: 2})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if page1.NextCursor == nil {
		t.Fatal("expected cursor")
	}

	newerID := insertArticleAt(t, db, authorID, "brand-new", base.Add(3*time.Minute))

	page2, err := svc.List(context.Background(), appsvc.ListRequest{Limit: 2, Cursor: *page1.NextCursor})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	for _, a := range page2.Articles {
		if a.ID == newerID {
			t.Fatal("D-04 violated: article newer than cursor appeared on the next page")
		}
		for _, first := range page1.Articles {
			if a.ID == first.ID {
				t.Errorf("duplicate id %d on page 2", a.ID)
			}
		}
	}
}

func TestList_Integration_AuthorFilterIncludesDuplicateNames(t *testing.T) {
	db := setupDB(t)
	isolateArticles(t, db)
	firstBudiID := seedAuthor(t, db, "Budi")
	secondBudiID := seedAuthor(t, db, "  BUDI  ")
	sitiID := seedAuthor(t, db, "Siti")
	svc := buildSvc(db)

	base := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Hour)
	firstArticleID := insertArticleAt(t, db, firstBudiID, "first-budi", base)
	secondArticleID := insertArticleAt(t, db, secondBudiID, "second-budi", base.Add(time.Minute))
	insertArticleAt(t, db, sitiID, "siti", base.Add(2*time.Minute))

	result, err := svc.List(context.Background(), appsvc.ListRequest{Author: "  bUdI  ", Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Articles) != 2 {
		t.Fatalf("len = %d, want 2", len(result.Articles))
	}
	if result.Articles[0].ID != secondArticleID || result.Articles[1].ID != firstArticleID {
		t.Errorf("got ids %d, %d; want %d, %d", result.Articles[0].ID, result.Articles[1].ID, secondArticleID, firstArticleID)
	}
}

func TestList_Integration_AuthorNotFoundReturnsEmpty(t *testing.T) {
	db := setupDB(t)
	isolateArticles(t, db)
	authorID := seedAuthor(t, db, "Known Author")
	insertArticleAt(t, db, authorID, "article", time.Now().UTC().Add(-time.Hour))
	svc := buildSvc(db)

	result, err := svc.List(context.Background(), appsvc.ListRequest{Author: "Unknown Author", Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Articles) != 0 || result.HasMore || result.NextCursor != nil {
		t.Errorf("unexpected result for missing author: %+v", result)
	}
}

func TestList_Integration_AuthorFilterPreservesAccents(t *testing.T) {
	db := setupDB(t)
	isolateArticles(t, db)
	accentedID := seedAuthor(t, db, "Büdi")
	plainID := seedAuthor(t, db, "Budi")
	svc := buildSvc(db)

	base := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Hour)
	accentedArticleID := insertArticleAt(t, db, accentedID, "accented", base)
	insertArticleAt(t, db, plainID, "plain", base.Add(time.Minute))

	result, err := svc.List(context.Background(), appsvc.ListRequest{Author: "Büdi", Limit: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Articles) != 1 || result.Articles[0].ID != accentedArticleID {
		t.Errorf("accented filter returned %+v, want only article %d", result.Articles, accentedArticleID)
	}
}

func TestList_Integration_AuthorFilterPagination(t *testing.T) {
	db := setupDB(t)
	isolateArticles(t, db)
	firstBudiID := seedAuthor(t, db, "Budi")
	secondBudiID := seedAuthor(t, db, "BUDI")
	sitiID := seedAuthor(t, db, "Siti")
	svc := buildSvc(db)

	base := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Hour)
	for i := 0; i < 5; i++ {
		authorID := firstBudiID
		if i%2 == 1 {
			authorID = secondBudiID
		}
		insertArticleAt(t, db, authorID, "budi-"+string(rune('1'+i)), base.Add(time.Duration(i)*time.Minute))
	}
	insertArticleAt(t, db, sitiID, "siti", base.Add(6*time.Minute))

	page1, err := svc.List(context.Background(), appsvc.ListRequest{Author: " BUDI ", Limit: 2})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if !page1.HasMore || page1.NextCursor == nil {
		t.Fatal("page1 should have next_cursor")
	}
	page2, err := svc.List(context.Background(), appsvc.ListRequest{Author: "budi", Limit: 2, Cursor: *page1.NextCursor})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	page3, err := svc.List(context.Background(), appsvc.ListRequest{Author: "Budi", Limit: 2, Cursor: *page2.NextCursor})
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	if len(page1.Articles) != 2 || len(page2.Articles) != 2 || len(page3.Articles) != 1 {
		t.Fatalf("page lengths = %d, %d, %d; want 2, 2, 1", len(page1.Articles), len(page2.Articles), len(page3.Articles))
	}
	if page3.HasMore || page3.NextCursor != nil {
		t.Error("last page must not have next_cursor")
	}

	seen := map[int64]bool{}
	for _, page := range [][]*domain.Article{page1.Articles, page2.Articles, page3.Articles} {
		assertNewestFirst(t, page)
		for _, a := range page {
			if seen[a.ID] {
				t.Errorf("duplicate article id %d across pages", a.ID)
			}
			seen[a.ID] = true
		}
	}
	if len(seen) != 5 {
		t.Errorf("unique ids = %d, want 5", len(seen))
	}
}
