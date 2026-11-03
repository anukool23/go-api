package middleware

import (
	"github/com/anukool23/olx-api/internal/httpx"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	trustedHeader   = "X-Real-IP"
	cleanupInterval = time.Minute * 2
	bucketTtl       = time.Minute * 10
)

type bucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type limiter struct {
	mu    sync.Mutex
	items map[string]*bucket
	limit rate.Limit
	burst int
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	item, ok := l.items[key]
	if !ok {
		item = &bucket{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.items[key] = item
	}
	item.lastSeen = time.Now()
	return item.limiter.Allow()

}

func RateLimit(logger *slog.Logger, limit rate.Limit, burst int) func(next http.Handler) http.Handler {
	//initilization
	l := limiter{
		items: make(map[string]*bucket),
		limit: limit,
		burst: burst,
	}
	//cleanum go routine
	go l.cleanup()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			//Executes on every requests
			userIP := clientIP(r, trustedHeader)
			if !l.allow(userIP) {
				logger.Info("RateLimit:allow", "key", userIP, "user_ip", userIP, "path", r.URL.Path, "request_id", RequestIdFromContext(r.Context()))
				retryAfter := "1"
				if limit > 0 && limit < 1 {
					retryAfter = strconv.Itoa(int(math.Ceil(float64(1 / limit))))
				}
				w.Header().Set("Retry-After", retryAfter)
				logger.Info("RateLimit:Header", "key", userIP, "user_ip", userIP, "path", r.URL.Path, "request_id", RequestIdFromContext(r.Context()))
				httpx.Error(w, http.StatusTooManyRequests, "Too many requests, Try after sometime", httpx.CodeRateLimitExceeded)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request, trustedHeader string) string {
	if trustedHeader != "" {
		return strings.TrimSpace(r.Header.Get(trustedHeader))
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func (l *limiter) cleanup() {
	ticker := time.NewTicker(cleanupInterval)
	l.mu.Lock()
	defer ticker.Stop()
	for now := range ticker.C {
		for key, item := range l.items {
			if now.Sub(item.lastSeen) > bucketTtl {
				delete(l.items, key)
			}
		}
	}
	l.mu.Unlock()
}
