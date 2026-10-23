package handlers

import (
	"encoding/json"
	"fmt"
	"github/com/anukool23/olx-api/internal/httpx"
	"github/com/anukool23/olx-api/internal/middleware"
	"github/com/anukool23/olx-api/internal/storage"
	"log/slog"
	"net/http"
	"time"
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
	uploads := make([]PreSignUpload, 0, len(req.Files))
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
		key := mintUploadKey(userId, ext)
		url, err := uh.store.PreSignUpload(ctx, key, f.ContentType, f.SizeBytes, preSignTTL)
		if err != nil {
			log.Error("UploadHandler:Presign:PreSignUpload:Failed to presign upload request", "error", err, "key", key)
			httpx.Error(w, http.StatusInternalServerError, "Something went wrong", httpx.CodeInternalError)
			return
		}
		uploads = append(uploads, PreSignUpload{
			UploadURL: url,
			ObjectKey: key,
			ExpiresAt: time.Now().Add(preSignTTL),
		})
	}
	log.Info("UploadHandler:Presign:Presigned upload urls generated successfully", "Total files", len(uploads))
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(uploads); err != nil {
		log.Error("UploadHandler:Presign:EncodingResponse:Failed to encode the response", "error", err)
		httpx.Error(w, http.StatusInternalServerError, "Something went wrong", httpx.CodeInternalError)
		return
	}
	w.WriteHeader(http.StatusAccepted)

	_ = json.NewEncoder(w).Encode(PreSignResponse{Uploads: uploads})
}
