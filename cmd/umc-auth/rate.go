package main

import (
	"net/http"
	"net/netip"
	"sync"
	"time"
)

// slidingWindow is a small in-process limiter. It is intentionally local to
// one auth container; use a shared limiter if you run multiple replicas.
type slidingWindow struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

func newSlidingWindow(limit int, window time.Duration) *slidingWindow {
	return &slidingWindow{hits: make(map[string][]time.Time), limit: limit, window: window}
}

func (l *slidingWindow) Check(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-l.window)
	values := l.hits[key]
	first := 0
	for first < len(values) && !values[first].After(cutoff) {
		first++
	}
	if first > 0 {
		l.hits[key] = values[first:]
	}
	return len(l.hits[key]) < l.limit
}

func (l *slidingWindow) Record(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits[key] = append(l.hits[key], now)
	if len(l.hits) > 10_000 {
		cutoff := now.Add(-l.window)
		for key, values := range l.hits {
			if len(values) == 0 || !values[len(values)-1].After(cutoff) {
				delete(l.hits, key)
			}
		}
	}
}

// rateKey aggregates IPv6 clients by /64 so address rotation within a subnet
// does not bypass per-client limits.
func rateKey(ip string) string {
	address, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	if address.Is6() {
		if prefix, err := address.Prefix(64); err == nil {
			return prefix.String()
		}
	}
	return address.String()
}

func (a *App) allowLimited(limiter *slidingWindow, r *http.Request) bool {
	key := rateKey(clientIP(r, a.cfg.TrustProxyHeaders))
	now := time.Now()
	if !limiter.Check(key, now) {
		return false
	}
	limiter.Record(key, now)
	return true
}
