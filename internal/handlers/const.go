package handlers

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	maximumFilesCountInRequest = 10
	maxImageBytes              = 5 * 1024 * 1024 //bytes
	uploadPrefix               = "uploads"
	preSignTTL                 = 5 * time.Minute
)

var allowedContentTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

func mintUploadKey(userId uuid.UUID, ext string) string {
	return fmt.Sprintf("%s/%s/%s.%s", uploadPrefix, userId, uuid.New(), ext)
}
