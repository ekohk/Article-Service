package metrics

import (
	"testing"
	"time"
)

func TestRegistrySnapshot(t *testing.T) {
	var r Registry
	r.IncRequest(200)
	r.IncRequest(500)
	r.IncCacheHit()
	r.IncCacheMiss()
	r.ObserveSearch(2 * time.Millisecond)
	r.SetOutboxPending(3)

	s := r.Snapshot()
	if s.Requests != 2 || s.RequestErrors != 1 || s.CacheHits != 1 ||
		s.CacheMisses != 1 || s.SearchCount != 1 ||
		s.SearchLatencyNS != int64(2*time.Millisecond) || s.OutboxPending != 3 {
		t.Fatalf("snapshot = %+v", s)
	}
}
