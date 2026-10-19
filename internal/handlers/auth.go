package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github/com/anukool23/olx-api/internal/httpx"
	"github/com/anukool23/olx-api/internal/middleware"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

type users struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Password  int64     `json:"password"`
	CreatedAt time.Time `json:"created_at"`
}

type AuthHandler struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewAuthHandler(db *sql.DB, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{
		db:     db,
		logger: logger,
	}
}

func (ah AuthHandler) SignUp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestId := middleware.RequestIdFromContext(ctx)
	log := ah.logger.With("requestId", requestId)
	var req SignUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("AuthHandler:Signup:Failed to decode", "err", err)
		httpx.Error(w, http.StatusUnprocessableEntity, "Request Validation Failed", httpx.CodeValidationError)
		return
	}
	if err := req.Validate(); err != nil {
		log.Error("AuthHandler:Singup:Request Validation Failed", "err", err)
		var verr ValidationError
		errors.As(err, &verr)
		httpx.ValidationError(w, http.StatusUnprocessableEntity, err.Error(), httpx.CodeValidationError, verr.Field)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		log.Error("AuthHandler:Singup:Password hashing failed", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Internal server error", httpx.CodeInternalError)
		return
	}

	row := ah.db.QueryRowContext(ctx, `INSERT INTO users (name, email, password) VALUES ($1,$2,$3) RETURNING id, created_at`, req.Name, req.Email, hashedPassword)
	var outputResponse SignUpResponse
	err = row.Scan(&outputResponse.ID, &outputResponse.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			log.Error("AuthHandler:Signup:QueryRowContext:Email already exists", "err", err)
			httpx.Error(w, http.StatusConflict, "Email already exists", httpx.CodeConflict)
			return
		}
		log.Error("AuthHandler:Signup:QueryRowContext:Failed to insert data into db", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Internal server error", httpx.CodeInternalError)
		return
	}
	log.Info("AuthHandler:Signup: User created successfully", "requestId", requestId, "user_id", outputResponse.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	_ = json.NewEncoder(w).Encode(outputResponse)
}
