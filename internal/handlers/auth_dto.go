package handlers

import (
	"net/mail"
	"strings"
)

type SignUpRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type SignUpResponse struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
}

func (req SignUpRequest) Validate() error {
	if strings.TrimSpace(req.Name) == "" {
		return ValidationError{Field: "name", Message: "Name is required"}
	}
	if strings.TrimSpace(req.Email) == "" {
		return ValidationError{Field: "email", Message: "Email is required"}
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		return ValidationError{Field: "email", Message: "Email is invalid"}
	}
	if strings.TrimSpace(req.Password) == "" {
		return ValidationError{Field: "password", Message: "password is required"}
	}
	if len(req.Password) < 8 {
		return ValidationError{Field: "password", Message: "password must be at least 8 chars"}
	}
	return nil
}
