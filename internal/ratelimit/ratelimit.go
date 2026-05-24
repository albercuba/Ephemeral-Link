package ratelimit

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type bucket struct {
	count int
	reset time.Time
}

type Limiter struct {
	mu          sync.Mutex
	perMinute   int
	buckets     map[string]bucket
	lastCleanup time.Time
}

func New(perMinute int) *Limiter {
	if perMinute <= 0 {
		perMinute = 60
	}
	return &Limiter{perMinute: perMinute, buckets: map[string]bucket{}, lastCleanup: time.Now()}
}

func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		now := time.Now()

		l.mu.Lock()
		l.cleanupExpiredLocked(now)
		b := l.buckets[ip]
		if now.After(b.reset) {
			b = bucket{reset: now.Add(time.Minute)}
		}
		b.count++
		l.buckets[ip] = b
		allow := b.count <= l.perMinute
		l.mu.Unlock()

		if !allow {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *Limiter) cleanupExpiredLocked(now time.Time) {
	if now.Sub(l.lastCleanup) < time.Minute {
		return
	}
	for ip, b := range l.buckets {
		if now.After(b.reset) {
			delete(l.buckets, ip)
		}
	}
	l.lastCleanup = now
}

func clientIP(r *http.Request) string {
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		return strings.TrimSpace(strings.Split(x, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
