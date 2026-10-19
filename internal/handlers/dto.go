package handlers

import (
	"fmt"
	"strings"
)

type CreateListingRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Price       int64  `json:"price"`
	City        string `json:"city"`
}

type CreateListingResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
}

type ValidationError struct {
	Field   string
	Message string
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
		return ValidationError{Field: "Description", Message: "Length should not be greater than 200 chars"}
	}
	if req.Price <= 0 {
		return ValidationError{Field: "Price", Message: "Price should be greater than zero"}
	}
	return nil
}
