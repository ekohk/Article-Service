package metrics

import (
	"sync/atomic"
	"time"
)

// Registry contains process-local counters for the service observability
// requirements. It intentionally has no external metrics dependency.
type Registry struct {
	requests      atomic.Uint64
	requestErrors atomic.Uint64
	cacheHits     atomic.Uint64
	cacheMisses   atomic.Uint64
	searchCount   atomic.Uint64
	searchLatency atomic.Int64
	outboxPending atomic.Int64
}

type Snapshot struct {
	Requests        uint64
	RequestErrors   uint64
	CacheHits       uint64
	CacheMisses     uint64
	SearchCount     uint64
	SearchLatencyNS int64
	OutboxPending   int64
}

func (r *Registry) IncRequest(status int) {
	r.requests.Add(1)
	if status >= 400 {
		r.requestErrors.Add(1)
	}
}

func (r *Registry) IncCacheHit()  { r.cacheHits.Add(1) }
func (r *Registry) IncCacheMiss() { r.cacheMisses.Add(1) }
func (r *Registry) ObserveSearch(d time.Duration) {
	r.searchCount.Add(1)
	r.searchLatency.Add(d.Nanoseconds())
}
func (r *Registry) SetOutboxPending(n int64) { r.outboxPending.Store(n) }

func (r *Registry) Snapshot() Snapshot {
	return Snapshot{
		Requests:        r.requests.Load(),
		RequestErrors:   r.requestErrors.Load(),
		CacheHits:       r.cacheHits.Load(),
		CacheMisses:     r.cacheMisses.Load(),
		SearchCount:     r.searchCount.Load(),
		SearchLatencyNS: r.searchLatency.Load(),
		OutboxPending:   r.outboxPending.Load(),
	}
}
