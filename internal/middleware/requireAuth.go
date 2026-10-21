package middleware

import (
	"context"
	"errors"
	"github/com/anukool23/olx-api/internal/httpx"
	"log/slog"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	authorization = "Authorization"
)

type AuthClaims struct {
	jwt.RegisteredClaims
}

func RequireAuth(logger *slog.Logger, secret string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			log := logger.With("request-id", RequestIdFromContext(ctx))
			authHeader := r.Header.Get(authorization)
			if authHeader == "" {
				log.Error("Middleware:RequireAuth:Authorization header missing")
				httpx.Error(w, http.StatusUnauthorized, "Authorization is required", httpx.CodeUnauthorized)
				return
			}
			raw, ok := strings.CutPrefix(authHeader, "Bearer ")
			if !ok || raw == "" {
				log.Error("Middleware:RequireAuth:Token is missing")
				httpx.Error(w, http.StatusUnauthorized, "Invalid token or token is missing", httpx.CodeUnauthorized)
				return
			}
			var claims AuthClaims
			_ , err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
				_, ok := t.Method.(*jwt.SigningMethodHMAC)
				if !ok {
					return nil, errors.New("Unexepcted Signing Method")
				}
				return []byte(secret), nil
			}, jwt.WithValidMethods([]string{"HS256"}))
			if err != nil {
				log.Info("Middleware:RequireAuth:Token Expired or Invalid token", "error", err)
				httpx.Error(w, http.StatusUnauthorized, "Token expired or Invalid token", httpx.CodeUnauthorized)
				return
			}
			userID, err := uuid.Parse(claims.Subject)
			if err != nil {
				log.Error("Middleware:RequireAuth:Invalid user ID in token", "error", err)
				httpx.Error(w, http.StatusUnauthorized, "Token is invalid or Expired", httpx.CodeUnauthorized)
				return
			}
			ctxWithUSerID := context.WithValue(ctx,userIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctxWithUSerID))
		})
	}
}

func UserIdFromContext(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(userIDKey).(uuid.UUID)
	return userID, ok
}
