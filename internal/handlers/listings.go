package handlers

import (
	"database/sql"
	"encoding/json"
	"github/com/anukool23/olx-api/internal/httpx"
	"github/com/anukool23/olx-api/internal/middleware"
	"log"
	"log/slog"
	"net/http"
	"time"
)

type listing struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Price       int64     `json:"price"`
	City        string    `json:"city"`
	CreatedAt   time.Time `json:"created_at"`
}

type ListingHandler struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewListingHandler(db *sql.DB, logger *slog.Logger) *ListingHandler {
	return &ListingHandler{
		db:     db,
		logger: logger,
	}
}

func (lh ListingHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestId := middleware.RequestIdFromContext(ctx)
	rows, err := lh.db.QueryContext(ctx,
		`SELECT id, title, description, price, city, created_at 
				FROM listings
				ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		lh.logger.Error("List:QueryContext: ", "requestId", requestId, "err", err)
		http.Error(w, "Failed to fetch listings", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	listings := []listing{}
	for rows.Next() {
		var l listing
		if err := rows.Scan(&l.ID, &l.Title, &l.Description, &l.Price, &l.City, &l.CreatedAt); err != nil {
			log.Printf("rows.scan : Failed to scan listing: %v", err)
			http.Error(w, "Failed to fetch listings", http.StatusInternalServerError)
			return
		}
		listings = append(listings, l)
	}
	if err := rows.Err(); err != nil {
		log.Printf("rows.err : Failed to iterate over listings: %v", err)
		http.Error(w, "Failed to fetch listings", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(listings)
}

func (lh ListingHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()
	requestId := middleware.RequestIdFromContext(ctx)
	_, err := lh.db.ExecContext(ctx, `DELETE FROM listings WHERE id = $1`, id)
	if err != nil {
		lh.logger.Error("Delete Failed", "requestId", requestId, "listing id", id, "err", err)
		log.Printf("db.Exec:delete %v", err)
		// http.Error(w, "Internal Error", http.StatusInternalServerError)
		httpx.Error(w, http.StatusInternalServerError, "Failed to delete listing", httpx.CodeInternalError)
		return
	}
	w.WriteHeader(http.StatusNoContent)

}

func (lh ListingHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestId := middleware.RequestIdFromContext(ctx)
	var req CreateListingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		lh.logger.Error("ListingHandler:Create:Failed to decode", "requestId", requestId, "err", err)
		httpx.Error(w, http.StatusBadRequest, "Request Validation Failed", httpx.CodeValidationError)
		return
	}
	row := lh.db.QueryRowContext(ctx, `INSERT INTO listings (title, description, price, city) VALUES ($1,$2,$3,$4) RETURNING id, title,created_at`, req.Title, req.Description, req.Price, req.City)
	var outputResponse CreateListingResponse
	if err := row.Scan(&outputResponse.ID, &outputResponse.Title, &outputResponse.CreatedAt); err != nil {
		lh.logger.Error("ListingHandler:Create:QueryRowContext:Failed to insert data into db", "requestId", requestId, "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Internal server error", httpx.CodeInternalError)
		return
	}
	lh.logger.Info("ListingHandler:Create: Listing created successfully", "requestId", requestId, "listing_id", outputResponse.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	_ = json.NewEncoder(w).Encode(outputResponse)
}
