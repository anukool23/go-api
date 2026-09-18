package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type ctxKey int

const (
	requestIdKey ctxKey = iota
)
const (
	requestId = "X-Request-Id"
)

func RequestId(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqId := r.Header.Get(requestId)
		if reqId == "" {
			reqId = uuid.NewString()
		}
		w.Header().Add(requestId, reqId)
		ctx := context.WithValue(r.Context(), requestIdKey,reqId)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequestIdFromContext(ctx context.Context) string {
	requestId := ctx.Value(requestIdKey).(string)
	return requestId
}