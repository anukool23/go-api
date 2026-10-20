package middleware

type ctxKey int

const (
	requestIdKey ctxKey = iota
	userIDKey
)