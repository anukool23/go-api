package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github/com/anukool23/olx-api/internal/httpx"
	"github/com/anukool23/olx-api/internal/jobs"
	"github/com/anukool23/olx-api/internal/middleware"
	"github/com/anukool23/olx-api/internal/storage"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const (
	defaultListingLimit = 20
	maxtListingLimit    = 50
)

const queryBeforeCursor = `
	SELECT l.id, l.title, l.description, l.price, l.city, l.status, l.created_at, l.user_id, i.id as imageId, i.object_key, i.position
	FROM listings l
	LEFT JOIN images i ON i.listing_id = l.id
	WHERE l.id IN (
		SELECT id FROM listings l2 WHERE l2.status = 'ready'
		ORDER BY l2.created_at DESC, l2.id DESC LIMIT $1
	)
	ORDER BY l.created_at DESC, l.id DESC, i.position
	`

const queryWithCursor = `
	SELECT l.id, l.title, l.description, l.price, l.city, l.status, l.created_at, l.user_id, i.id as imageId, i.object_key, i.position
	FROM listings l
	LEFT JOIN images i ON i.listing_id = l.id
	WHERE l.id IN (
		SELECT id FROM listings l2 WHERE l2.status = 'ready' AND (l2.created_at, l2.id) < ($1,$2)
		ORDER BY l2.created_at DESC, l2.id DESC LIMIT $3
	)
	ORDER BY l.created_at DESC, l.id DESC, i.position
	`

type listing struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Price       int64     `json:"price"`
	City        string    `json:"city"`
	Userid      uuid.UUID `json:"user_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type imageRow struct {
	ID        uuid.UUID
	ObjectKey string
	Position  int16
}

type ListingHandler struct {
	db      *sql.DB
	logger  *slog.Logger
	storage *storage.Client
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
	// For testing purpose only, uncomment the below line to simulate a delay in response
	// lh.db.QueryContext(ctx, `SELECT pg_sleep(10)`)

	requestId := middleware.RequestIdFromContext(ctx)
	log := lh.logger.With("requestId", requestId)

	filters, err := parseListingFilters(r.URL.Query())
	if err != nil {
		var verr *ValidationError
		if errors.As(err, &verr) {
			httpx.ValidationError(w, http.StatusBadRequest, err.Error(), httpx.CodeValidationError, verr.Field)
			return
		}
		httpx.ValidationError(w, http.StatusBadRequest, err.Error(), httpx.CodeValidationError, "")
		return
	}

	limit, err := lh.parseListingLimit(r.URL.Query().Get("limit"))
	if err != nil {
		log.Error(`ListingHandler:List:parseListingLimit:Failed to parse limit`, "requestId", requestId, "err", err)
		httpx.ValidationError(w, http.StatusBadRequest, err.Error(), httpx.CodeBadRequest, "limit")
		return
	}
	after := r.URL.Query().Get("after")
	rows, err := lh.queryListings(ctx, filters, after, limit+1)
	if err != nil {
		log.Error("ListingHandler:List:queryListings:Failed to query listings", "requestId", requestId, "err", err)
		httpx.Error(w, http.StatusInternalServerError, "something went wrong", httpx.CodeInternalError)
		return
	}
	defer rows.Close()

	listings, err := lh.collapse(rows)
	if errors.Is(err, errInvalidCursor) {
		log.Error("ListingHandler:List:collapse:Invalid cursor", "requestId", requestId, "err", err)
		httpx.ValidationError(w, http.StatusBadRequest, "Invalid cursor", httpx.CodeBadRequest, "after")
		return
	}
	if err != nil {
		log.Error("ListingHandler:List:collapse:Failed to collapse rows", "requestId", requestId, "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Failed to fetch listings", httpx.CodeInternalError)
		return
	}
	out := ListListingResponse{
		Data: listings,
	}
	if len(listings) > limit {
		out.Data = listings[:limit]
		lastRow := out.Data[len(out.Data)-1]
		id, err := uuid.Parse(lastRow.ID)
		if err != nil {
			log.Error("ListingHandler:List:Failed to parse listing id", "requestId", requestId, "listing_id", lastRow.ID, "err", err)
			httpx.Error(w, http.StatusInternalServerError, "Something went wrong", httpx.CodeInternalError)
			return
		}

		cursor, err := encodeCursor(listingCursor{
			ID:        id,
			CreatedAt: lastRow.CreatedAt,
			Version:   cursorVersion,
		})
		if err != nil {
			log.Error("ListingHandler:List:Failed to encode cursor", "requestId", requestId, "err", err)
			httpx.Error(w, http.StatusInternalServerError, "Something went wrong", httpx.CodeInternalError)
			return
		}

		out = ListListingResponse{
			Data:       out.Data,
			NextCursor: &cursor,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(out)
}

func (lh ListingHandler) queryListings(ctx context.Context, filters listingFilters, after string, limit int) (*sql.Rows, error) {
	requestId := middleware.RequestIdFromContext(ctx)
	log := lh.logger.With("requestId", requestId, "after", after, "limit", limit)
	var cur *listingCursor
	if after != "" {
		cursor, err := decodeCursor(after)
		if err != nil {
			log.Error("ListingHandler:queryListings:Failed to decode cursor", "after", after, "err", err)
			return nil, err
		}
		cur = &cursor
	}
	query, args := buildListingQuery(filters, cur, limit)
	log.Info("ListingHandler:queryListings:Cursor provided, fetching next page", "cursor", cur, "limit", limit)
	return lh.db.QueryContext(ctx, query, args...)
}

func (lh ListingHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()
	requestId := middleware.RequestIdFromContext(ctx)
	userId, ok := middleware.UserIdFromContext(ctx)
	if !ok {
		lh.logger.Error("ListingHandler:Delete:Failed to get user id from context", "requestId", requestId)
		httpx.Error(w, http.StatusInternalServerError, "Something went wrong", httpx.CodeInternalError)
		return
	}
	_, err := lh.db.ExecContext(ctx, `DELETE FROM listings WHERE id = $1 AND user_id = $2`, id, userId)
	if err != nil {
		lh.logger.Error("ListingHandler:Delete:Failed to delete listing", "requestId", requestId, "listing id", id, "err", err)
		// http.Error(w, "Internal Error", http.StatusInternalServerError)
		httpx.Error(w, http.StatusInternalServerError, "Failed to delete listing", httpx.CodeInternalError)
		return
	}
	w.WriteHeader(http.StatusNoContent)

}

func (lh ListingHandler) ListingDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()
	requestId := middleware.RequestIdFromContext(ctx)
	log := lh.logger.With("requestId", requestId)
	rows, err := lh.db.QueryContext(ctx, `
	SELECT
        l.id,
        l.title,
        l.description,
        l.price,
        l.city,
        l.status,
        l.created_at,
        l.user_id,
        i.id AS imageId,
        i.object_key,
        i.position
    FROM listings l
    LEFT JOIN images i ON i.listing_id = l.id
    WHERE l.id = $1
    ORDER BY l.created_at DESC, l.id DESC, i.position`, id)
	if err != nil {
		log.Error("ListingHandler:ListingDetail:Failed to query listing", "requestId", requestId, "err", err)
		httpx.Error(w, http.StatusInternalServerError, "something went wrong", httpx.CodeInternalError)
		return
	}
	defer rows.Close()

	listings, err := lh.collapse(rows)
	if err != nil {
		log.Error("ListingHandler:ListingDetail:Failed to collapse rows", "requestId", requestId, "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Failed to fetch listings", httpx.CodeInternalError)
		return
	}
	if len(listings) == 0 {
		log.Info("ListingHandler:ListingDetail:Listing not found", "requestId", requestId, "listing_id", id)
		httpx.Error(w, http.StatusNotFound, "Listing not found", httpx.CodeNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(listings[0])

}

func (lh ListingHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestId := middleware.RequestIdFromContext(ctx)
	log := lh.logger.With("requestId", requestId)
	userId, ok := middleware.UserIdFromContext(ctx)
	if !ok {
		log.Error("ListingHandler:Create:Failed to get user id from context", "requestId", requestId)
		httpx.Error(w, http.StatusInternalServerError, "Something went wrong", httpx.CodeInternalError)
		return
	}
	var req CreateListingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("ListingHandler:Create:Failed to decode", "requestId", requestId, "err", err)
		httpx.Error(w, http.StatusUnprocessableEntity, "Request Validation Failed", httpx.CodeValidationError)
		return
	}
	if err := req.Validate(); err != nil {
		log.Error("ListingHandler:Create:Request Validation Failed", "requestId", requestId, "err", err)
		var verr ValidationError
		errors.As(err, &verr)
		httpx.ValidationError(w, http.StatusUnprocessableEntity, err.Error(), httpx.CodeValidationError, verr.Field)
		return
	}
	listingId := uuid.New()
	sources := make([]jobs.ImageSource, 0, len(req.ImageKeys))
	imagekeys := make([]string, 0, len(req.ImageKeys))
	for _, key := range req.ImageKeys {
		imageId, _, err := parseUploadKeys(key, userId)
		if err != nil {
			log.Error("ListingHandler:Create:Failed to parse upload key", "requestId", requestId, "err", err)
			httpx.Error(w, http.StatusUnprocessableEntity, "Invalid image key", httpx.CodeValidationError)
			return
		}
		imagekeys = append(imagekeys, mintImagekey(imageId))
		sources = append(sources, jobs.ImageSource{
			Uploadkey: key,
			ObjectKey: mintFinalObjectKey(listingId, imageId),
		})
	}

	for _, s := range sources {
		log.Info("ListingHandler:Create:Image source", "requestId", requestId, "upload_key", s.Uploadkey, "object_key", s.ObjectKey)
		contentLength, contentType, err := lh.storage.Head(ctx, s.Uploadkey)
		if errors.Is(err, storage.ErrNotFound) {
			log.Error("ListingHandler:Create:ErrNotFound:Failed to parse upload key in source", "requestId", requestId, "err", err)
			httpx.ValidationError(w, http.StatusInternalServerError, "No object found at Object key", httpx.CodeInternalError, "image_keys")
			return
		}
		if err != nil {
			log.Error("ListingHandler:Create:Failed to parse upload key in source", "requestId", requestId, "err", err)
			httpx.ValidationError(w, http.StatusBadRequest, "No object found at Object key", httpx.CodeBadRequest, "image_keys")
			return
		}
		if contentLength <= 0 || contentLength > maxImageBytes {
			log.Error("ListingHandler:Create:Image size is out of range", "requestId", requestId)
			httpx.ValidationError(w, http.StatusBadRequest, "Uploaded image is out of size limit range", httpx.CodeValidationError, "image_keys")
			return
		}
		if _, ok := allowedContentTypes[contentType]; !ok {
			log.Error("ListingHandler:Create:Uploaded image content type mismatch", "requestId", requestId)
			httpx.ValidationError(w, http.StatusBadRequest, "Uploaded image content type mismatch", httpx.CodeValidationError, "image_keys")
			return
		}
	}

	tx, err := lh.db.BeginTx(ctx, nil)
	if err != nil {
		log.Error("ListingHandler:Create:BeginTx Failed to start transaction", "requestId", requestId, "error", err)
		httpx.Error(w, http.StatusInternalServerError, "Something went wrong", httpx.CodeInternalError)
		return
	}
	defer func() {
		_ = tx.Rollback()
	}()

	row := tx.QueryRowContext(ctx, `INSERT INTO listings (id, title, description, price, city, user_id) 
	VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, title, status, created_at`,
		listingId, req.Title, req.Description, req.Price, req.City, userId)

	var outputResponse CreateListingResponse
	if err := row.Scan(&outputResponse.ID, &outputResponse.Title, &outputResponse.Status, &outputResponse.CreatedAt); err != nil {
		log.Error("ListingHandler:Create:QueryRowContext:Failed to insert data into db", "requestId", requestId, "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Internal server error", httpx.CodeInternalError)
		return
	}

	for i, imageKey := range imagekeys {
		_, err := tx.ExecContext(ctx, `INSERT INTO images (listing_id, object_key, position) 
		VALUES ($1, $2, $3)`, listingId, imageKey, int16(i))
		if err != nil {
			log.Error("ListingHandler:Create:ExecContext:Insertion of image is failed", "requestId", "imageKey", imageKey, requestId, "err", err)
			httpx.Error(w, http.StatusInternalServerError, "Internal server error", httpx.CodeInternalError)
			return
		}
	}

	payload, err := json.Marshal(jobs.ProcessImagePayload{
		ListingId: listingId,
		Sources:   sources,
	})
	if err != nil {
		log.Error("ListingHandler:Create:Marshal:Prepration of jobs payload is failed", "requestId", requestId, "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Internal server error", httpx.CodeInternalError)
		return
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO jobs (kind, payload) VALUES ($1,$2)`, jobs.KindProcessListingImage, payload)

	if err != nil {
		log.Error("ListingHandler:Create:KindProcessListingImage:Insert of jobs payload is failed", "requestId", requestId, "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Internal server error", httpx.CodeInternalError)
		return
	}

	if err = tx.Commit(); err != nil {
		log.Error("ListingHandler:Create:Commit:commit failed", "requestId", requestId, "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Internal server error", httpx.CodeInternalError)
		return
	}

	log.Info("ListingHandler:Create: Listing created successfully", "requestId", requestId, "listing_id", outputResponse.ID)

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

func (lh ListingHandler) collapse(rows *sql.Rows) ([]GetListingResponse, error) {
	listings := []GetListingResponse{}
	images := map[string][]imageRow{}
	seen := make(map[string]struct{})
	for rows.Next() {
		var l GetListingResponse
		var imageId uuid.NullUUID
		var objKey sql.NullString
		var position sql.NullInt16
		if err := rows.Scan(&l.ID, &l.Title, &l.Description, &l.Price, &l.City, &l.Status, &l.CreatedAt, &l.Userid, &imageId, &objKey, &position); err != nil {
			lh.logger.Error("rows.scan : Failed to scan listing: %w", err)
			return nil, err
		}
		if _, ok := seen[l.ID]; !ok {
			seen[l.ID] = struct{}{}
			listings = append(listings, l)
		}
		if imageId.Valid && objKey.Valid && position.Valid {
			images[l.ID] = append(images[l.ID], imageRow{
				ID:        imageId.UUID,
				ObjectKey: objKey.String,
				Position:  position.Int16,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range listings {
		listings[i].Images = lh.toImageResponse(images[listings[i].ID])
	}
	return listings, nil
}

func (lh ListingHandler) toImageResponse(imageRow []imageRow) []ImageResponse {
	out := make([]ImageResponse, 0, len(imageRow))
	for _, img := range imageRow {
		out = append(out, ImageResponse{
			ID:        img.ID.String(),
			ObjectKey: img.ObjectKey,
			Position:  img.Position})
	}
	return out
}

func (lh ListingHandler) parseListingLimit(raw string) (int, error) {
	lh.logger.Info("The raw value is", "raw", raw)
	if raw == "" {
		return defaultListingLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("limit must be an integer")
	}
	if n < 1 || n > maxtListingLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", maxtListingLimit)
	}
	lh.logger.Info("The final value is", "n", n)
	return n, nil
}
