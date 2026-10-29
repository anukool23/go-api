package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github/com/anukool23/olx-api/internal/httpx"
	"github/com/anukool23/olx-api/internal/jobs"
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
	requestId := middleware.RequestIdFromContext(ctx)
	log := lh.logger.With("requestId", requestId)
	rows, err := lh.db.QueryContext(ctx, `
	SELECT l.id, l.title, l.description, l.price, l.city, l.status, l.created_at, l.user_id, i.id as imageId, i.object_key, i.position
	FROM listings l
	LEFT JOIN images i ON i.listing_id = l.id
	WHERE l.id IN (
		SELECT id FROM listings ORDER BY created_at DESC, id DESC LIMIT $1
	)
	ORDER BY l.created_at DESC, l.id DESC, i.position`, MaxMyListings)
	if err != nil {
		log.Error("my listings query error", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "something went wrong", httpx.CodeInternalError)
		return
	}
	defer rows.Close()

	listings, err := lh.collapse(rows)
	if err != nil {
		lh.logger.Error("Listing Handler : List : Collapse : Error from collapse", "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Failed to fetch listings", httpx.CodeInternalError)
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
	sources := make([]jobs.ImageSource, 0, len(req.ImageKeys))
	imagekeys := make([]string, 0, len(req.ImageKeys))
	for _, key := range req.ImageKeys {
		imageId, _, err := parseUploadKeys(key, userId)
		if err != nil {
			lh.logger.Error("ListingHandler:Create:Failed to parse upload key", "requestId", requestId, "err", err)
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

	tx, err := lh.db.BeginTx(ctx, nil)
	if err != nil {
		lh.logger.Error("ListingHandler:Create:BeginTx Failed to start transaction", "requestId", requestId, "error", err)
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
		lh.logger.Error("ListingHandler:Create:QueryRowContext:Failed to insert data into db", "requestId", requestId, "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Internal server error", httpx.CodeInternalError)
		return
	}

	for i, imageKey := range imagekeys {
		_, err := tx.ExecContext(ctx, `INSERT INTO images (listing_id, object_key, position) 
		VALUES ($1, $2, $3)`, listingId, imageKey, int16(i))
		if err != nil {
			lh.logger.Error("ListingHandler:Create:ExecContext:Insertion of image is failed", "requestId", "imageKey", imageKey, requestId, "err", err)
			httpx.Error(w, http.StatusInternalServerError, "Internal server error", httpx.CodeInternalError)
			return
		}
	}

	payload, err := json.Marshal(jobs.ProcessImagePayload{
		ListingId: listingId,
		Sources:   sources,
	})
	if err != nil {
		lh.logger.Error("ListingHandler:Create:Marshal:Prepration of jobs payload is failed", "requestId", requestId, "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Internal server error", httpx.CodeInternalError)
		return
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO jobs (kind, payload) VALUES ($1,$2)`, jobs.KindProcessListingImage, payload)

	if err != nil {
		lh.logger.Error("ListingHandler:Create:KindProcessListingImage:Insert of jobs payload is failed", "requestId", requestId, "err", err)
		httpx.Error(w, http.StatusInternalServerError, "Internal server error", httpx.CodeInternalError)
		return
	}

	if err = tx.Commit(); err != nil {
		lh.logger.Error("ListingHandler:Create:Commit:commit failed", "requestId", requestId, "err", err)
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

func (lh ListingHandler) collapse(rows *sql.Rows) ([]GetListingResponse, error) {
	listings := []GetListingResponse{}
	images := map[string][]imageRow{}
	seen := make(map[string]struct{})
	for rows.Next() {
		var l GetListingResponse
		var imageId uuid.NullUUID
		var objKey sql.NullString
		var position sql.NullInt16
		if err := rows.Scan(&l.ID, &l.Title, &l.Description, &l.Price, &l.City,&l.Status, &l.CreatedAt, &l.Userid, &imageId, &objKey, &position); err != nil {
			lh.logger.Error("rows.scan : Failed to scan listing: %v", err)
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
