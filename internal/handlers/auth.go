package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github/com/anukool23/olx-api/internal/config"
	"github/com/anukool23/olx-api/internal/httpx"
	"github/com/anukool23/olx-api/internal/middleware"
	"log/slog"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

var dummyHash = []byte("$2a$12$wI0Q6Z1J5F1G5F1G5F1G5eF1G5F1G5F1G5F1G5F1G5F1G5F1G5F1G")
var tokenTTL = 24 * time.Hour

type users struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Password  string    `json:"password"`
	CreatedAt time.Time `json:"created_at"`
}

type AuthHandler struct {
	db     *sql.DB
	logger *slog.Logger
	cfg config.Config
}

func NewAuthHandler(db *sql.DB, logger *slog.Logger, cfg config.Config) *AuthHandler {
	return &AuthHandler{
		db:     db,
		logger: logger,
		cfg: cfg,
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

func (ah AuthHandler) SignIn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestId := middleware.RequestIdFromContext(ctx)
	log := ah.logger.With("requestId", requestId)
	var req SignInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("AuthHandler:SignIn:Failed to decode", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Something went wrong", httpx.CodeInternalError)
		return
	}

	if err := req.Validate(); err != nil {
		log.Error("AuthHandler:SignIn:Request Validation Failed", "err", err)
		var verr *ValidationError
		errors.As(err, &verr)
		httpx.ValidationError(w, http.StatusUnprocessableEntity, err.Error(), httpx.CodeValidationError, verr.Field)
		return
	}
	var u users
	row := ah.db.QueryRowContext(ctx, "SELECT ID, email, password FROM users WHERE email = $1", req.Email)
	err := row.Scan(&u.ID, &u.Email, &u.Password)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password))
			log.Error("AuthHandler:SignIn:QueryRowContext:User not found", "err", err)
			httpx.Error(w, http.StatusUnauthorized, "Invalid credentials", httpx.CodeUnauthorized)
			return
		}
		log.Error("AuthHandler:SignIn:QueryRowContext:Failed to scan data", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Something went wrong", httpx.CodeInternalError)
		return
	}

	if err = bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(req.Password)); err != nil {
		log.Warn("AuthHandler:SignIn:Password comparison failed", "user_id", u.ID, "err", err)
		httpx.Error(w, http.StatusUnauthorized, "Invalid credentials", httpx.CodeUnauthorized)
		return
	}
	claims := jwt.RegisteredClaims{
		Subject:   u.ID,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	log.Debug("AuthHandler:SignIn:Token generated", "token", token)
	signed , err := token.SignedString([]byte(ah.cfg.JwtKey))
	if err != nil {
		log.Error("AuthHandler:SignIn:Token signing failed", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Something went wrong", httpx.CodeInternalError)
		return
	}
	log.Info("AuthHandler:SignIn:User signed in successfully", "requestId", requestId, "user_id", u.ID)

	out := SignInResponse{
		Token: signed,
		ExpiresIn: int64(tokenTTL.Seconds()),
	}
	log.Debug("User successfully logged in", "user_id",u.ID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}
