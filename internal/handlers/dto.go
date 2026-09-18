package handlers

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
