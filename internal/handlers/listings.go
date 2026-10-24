package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github/com/anukool23/olx-api/internal/httpx"
	"github/com/anukool23/olx-api/internal/middleware"
	"github/com/anukool23/olx-api/internal/storage"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type listing struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Price       int64     `json:"price"`
	City        string    `json:"city"`
	Userid      uuid.UUID `json:"user_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type ListingHandler struct {
	db      *sql.DB
	logger  *slog.Logger
	storage *storage.Client
}

type imageSource struct {
	Uploadkey string `json:"upload_key"` //path before processing (Uploaded path)
	ObjectKey string `json:"object_key"` //path after processing (Processed path)
}

func NewListingHandler(db *sql.DB, logger *slog.Logger, storage *storage.Client) *ListingHandler {
	return &ListingHandler{
		db:      db,
		logger:  logger,
		storage: storage,
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
	userId, ok := middleware.UserIdFromContext(ctx)
	if !ok {
		lh.logger.Error("ListingHandler:Create:Failed to get user id from context", "requestId", requestId)
		httpx.Error(w, http.StatusInternalServerError, "Something went wrong", httpx.CodeInternalError)
		return
	}
	_, err := lh.db.ExecContext(ctx, `DELETE FROM listings WHERE id = $1 AND user_id = $2`, id, userId)
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
	userId, ok := middleware.UserIdFromContext(ctx)
	if !ok {
		lh.logger.Error("ListingHandler:Create:Failed to get user id from context", "requestId", requestId)
		httpx.Error(w, http.StatusInternalServerError, "Something went wrong", httpx.CodeInternalError)
		return
	}
	var req CreateListingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		lh.logger.Error("ListingHandler:Create:Failed to decode", "requestId", requestId, "err", err)
		httpx.Error(w, http.StatusUnprocessableEntity, "Request Validation Failed", httpx.CodeValidationError)
		return
	}
	if err := req.Validate(); err != nil {
		lh.logger.Error("ListingHandler:Create:Request Validation Failed", "requestId", requestId, "err", err)
		var verr ValidationError
		errors.As(err, &verr)
		httpx.ValidationError(w, http.StatusUnprocessableEntity, err.Error(), httpx.CodeValidationError, verr.Field)
		return
	}
	listingId := uuid.New()
	sources := make([]imageSource, 0, len(req.ImageKeys))
	for _, key := range req.ImageKeys {
		imageId, ext, err := parseUploadKeys(key, userId)
		if err != nil {
			lh.logger.Error("ListingHandler:Create:Failed to parse upload key", "requestId", requestId, "err", err)
			httpx.Error(w, http.StatusUnprocessableEntity, "Invalid image key", httpx.CodeValidationError)
			return
		}
		sources = append(sources, imageSource{
			Uploadkey: key,
			ObjectKey: mintFinalObjectKey(listingId, imageId, ext),
		})
	}

	for _, s := range sources {
		lh.logger.Info("ListingHandler:Create:Image source", "requestId", requestId, "upload_key", s.Uploadkey, "object_key", s.ObjectKey)
		contentLength, contentType, err := lh.storage.Head(ctx, s.Uploadkey)
		if errors.Is(err, storage.ErrNotFound) {
			lh.logger.Error("ListingHandler:Create:ErrNotFound:Failed to parse upload key in source", "requestId", requestId, "err", err)
			httpx.ValidationError(w, http.StatusInternalServerError, "No object found at Object key", httpx.CodeInternalError, "image_keys")
			return
		}
		if err != nil {
			lh.logger.Error("ListingHandler:Create:Failed to parse upload key in source", "requestId", requestId, "err", err)
			httpx.ValidationError(w, http.StatusBadRequest, "No object found at Object key", httpx.CodeBadRequest, "image_keys")
			return
		}
		if contentLength <= 0 || contentLength > maxImageBytes {
			lh.logger.Error("ListingHandler:Create:Image size is out of range", "requestId", requestId)
			httpx.ValidationError(w, http.StatusBadRequest, "Uploaded image is out of size limit range", httpx.CodeValidationError, "image_keys")
			return
		}
		if _, ok := allowedContentTypes[contentType]; !ok {
			lh.logger.Error("ListingHandler:Create:Uploaded image content type mismatch", "requestId", requestId)
			httpx.ValidationError(w, http.StatusBadRequest, "Uploaded image content type mismatch", httpx.CodeValidationError, "image_keys")
			return
		}
		

	}
	row := lh.db.QueryRowContext(ctx, `INSERT INTO listings (id, title, description, price, city, user_id) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, title,created_at`, listingId, req.Title, req.Description, req.Price, req.City, userId)
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

func (lh ListingHandler) MyListings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestId := middleware.RequestIdFromContext(ctx)
	log := lh.logger.With("requestId", requestId)
	userId, ok := middleware.UserIdFromContext(ctx)
	if !ok {
		log.Error("ListingHandler:MyLisint:UserIdFromContext:Failed to extract userid from request")
		httpx.Error(w, http.StatusUnauthorized, "Something went wrong", httpx.CodeUnauthorized)
		return
	}
	rows, err := lh.db.QueryContext(ctx, `SELECT id, title, description, price, city, created_at 
				FROM listings WHERE user_id = $1`, userId)
	if err != nil {
		log.Error("ListingHandler:MyLisint:QueryContext:Error while fetching record from DB", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Internal server error", httpx.CodeInternalError)
		return
	}
	defer rows.Close()
	listings := []listing{}
	for rows.Next() {
		var l listing
		if err := rows.Scan(&l.ID, &l.Title, &l.Description, &l.Price, &l.City, &l.CreatedAt); err != nil {
			log.Error("ListingHandler:MyLisint:QueryContext:Error while iterating record from DB", "err", err)
			httpx.Error(w, http.StatusInternalServerError, "Internal Server error", httpx.CodeInternalError)
			return
		}
		listings = append(listings, l)
	}
	if err := rows.Err(); err != nil {
		log.Error("ListingHandler:MyLisint:QueryContext:Error while iterating record from DB", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Internal Server error", httpx.CodeInternalError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(listings)

}
