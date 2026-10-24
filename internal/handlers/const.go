package handlers

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maximumFilesCountInRequest = 10
	maxImageBytes              = 5 * 1024 * 1024 //bytes
	uploadPrefix               = "uploads"
	listingPrefix              = "listings"
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

func mintFinalObjectKey(listingId, imageId uuid.UUID, ext string) string {
	return fmt.Sprintf("%s/%s/%s.%s", listingPrefix, listingId, imageId, ext)
}

func parseUploadKeys(key string, userId uuid.UUID) (uuid.UUID, string,  error) {
	rest, ok := strings.CutPrefix(key, fmt.Sprintf("%s/%s/", uploadPrefix, userId))
	if !ok {
		return uuid.Nil, "",  errors.New("invalid key format")
	}
	if !strings.Contains(rest, "/") {
		return uuid.Nil,"",  errors.New("Object key has unexpected shape")
	}
	base, ext, ok := strings.Cut(rest, ".")
	if !ok || !allowedExt(ext) {
		return uuid.Nil, "", errors.New("Object key has unexpected shape or invalid extension")
	}
	imageId, err := uuid.Parse(base)
	if err != nil {
		return uuid.Nil, "", errors.New("Invalid image id in object key")
	}
	return imageId, ext, nil
}

func allowedExt(ext string) bool {
	for _, allowedExt := range allowedContentTypes {
		if ext == allowedExt {
			return true
		}
	}
	return false
}
