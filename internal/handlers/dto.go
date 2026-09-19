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
	ID string `json:"id"`
	Title       string `json:"title"`
	CreatedAt   string `json:"created_at"`
}

type ValidationError struct {
	Field string
	Message string
}

func (ve ValidationError) Error() string{
	return fmt.Sprintf("%s : %s", ve.Field, ve.Message)
}

func (req CreateListingRequest) Validate() error {
	if strings.TrimSpace(req.Title) == "" {
		return ValidationError{Field: "title", Message: "Title is required"}
	}
	return nil
}