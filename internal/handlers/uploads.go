package handlers

import (
	"encoding/json"
	"fmt"
	"github/com/anukool23/olx-api/internal/httpx"
	"github/com/anukool23/olx-api/internal/middleware"
	"github/com/anukool23/olx-api/internal/storage"
	"log/slog"
	"net/http"
)

type UploadHandler struct {
	logger *slog.Logger
	store  *storage.Client
}

func NewUploadHandler(logger *slog.Logger, store *storage.Client) *UploadHandler {
	return &UploadHandler{
		logger: logger,
		store:  store,
	}
}

func (uh UploadHandler) Presign(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestId := middleware.RequestIdFromContext(ctx)
	userId, ok := middleware.UserIdFromContext(ctx)
	log := uh.logger.With("requestId", requestId)
	if !ok {
		log.Error("UploadHandler:Presign:UserIdFromContext:Failed to extract userid from request")
		httpx.Error(w, http.StatusUnauthorized, "Something went wrong", httpx.CodeUnauthorized)
		return
	}
	var req PresignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UploadHandler:Presign:DecodingRequest:Failed to decode the request")
		httpx.Error(w, http.StatusBadRequest, "Invalid request body", httpx.CodeBadRequest)
		return
	}
	if len(req.Files) == 0 {
		log.Error("UploadHandler:Presign:FilesCheck:No files in request")
		httpx.Error(w, http.StatusBadRequest, "Invalid files or files is missing in request", httpx.CodeValidationError)
		return
	}
	if len(req.Files) > maximumFilesCountInRequest {
		log.Error("UploadHandler:Presign:FilesCheck:THe files count in request is more than limit", "Total files", len(req.Files))
		httpx.Error(w, http.StatusBadRequest, fmt.Sprintf("Total files cannot be more than %d images", maximumFilesCountInRequest), httpx.CodeBadRequest)
		return
	}
	for _, f := range req.Files {
		ext, ok := allowedContentTypes[f.ContentType]
		if !ok {
			log.Error("UploadHandler:Presign:allowedContentTypes:Incorrect image format", "ext", ext)
			httpx.Error(w, http.StatusBadRequest, "Only jpeg, jpg, png and webp are allowed", httpx.CodeBadRequest)
			return
		}
		if f.SizeBytes <= 0 || f.SizeBytes > maxImageBytes {
			log.Error("UploadHandler:Presign:allowedContentSize:Image size is not in limit", "Size", f.SizeBytes)
			httpx.Error(w, http.StatusBadRequest, fmt.Sprintf("Image size must be between 1 to %d bytes", maxImageBytes), httpx.CodeBadRequest)
			return
		}
		// key := mintUploadKey(userId, ext)

	}

	w.Write([]byte("Presign endpoint"))
}
