package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type CreateListingRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Price       int64    `json:"price"`
	City        string   `json:"city"`
	ImageKeys   []string `json:"image_keys"`
}

type CreateListingResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type ValidationError struct {
	Field   string
	Message string
}

type ImageResponse struct {
	ID        string `json:"id"`
	ObjectKey string `json:"object_key"`
	Position  int16  `json:"position"`
}

type GetListingResponse struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Price       int64           `json:"price"`
	City        string          `json:"city"`
	Status      string          `json:"status"`
	Userid      uuid.UUID       `json:"user_id"`
	Images      []ImageResponse `json:"images"`
	CreatedAt   time.Time       `json:"created_at"`
}

func (ve ValidationError) Error() string {
	return fmt.Sprintf("%s : %s", ve.Field, ve.Message)
}

func (req CreateListingRequest) Validate() error {
	if strings.TrimSpace(req.Title) == "" {
		return ValidationError{Field: "title", Message: "Title is required"}
	}
	if len(req.Title) > 200 {
		return ValidationError{Field: "title", Message: "Length should not be greater than 200 chars"}
	}
	if strings.TrimSpace(req.Description) == "" {
		return ValidationError{Field: "Description", Message: "Description is required"}
	}
	if len(req.Description) > 5000 {
		return ValidationError{Field: "Description", Message: "Length should not be greater than 5000 chars"}
	}
	if req.Price <= 0 {
		return ValidationError{Field: "Price", Message: "Price should be greater than zero"}
	}
	if strings.TrimSpace(req.City) == "" {
		return ValidationError{Field: "City", Message: "City is required"}
	}
	if len(req.City) > 100 {
		return ValidationError{Field: "City", Message: "Length should not be greater than 100 chars"}
	}
	if len(req.ImageKeys) == 0 {
		return ValidationError{Field: "ImageKeys", Message: "At least one image is required"}
	}
	if len(req.ImageKeys) > 10 {
		return ValidationError{Field: "ImageKeys", Message: "Maximum 10 images are allowed"}
	}
	for _, key := range req.ImageKeys {
		if strings.TrimSpace(key) == "" {
			return ValidationError{Field: "ImageKeys", Message: "Image key cannot be empty"}
		}
	}
	return nil
}
