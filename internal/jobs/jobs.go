package jobs

import "github.com/google/uuid"

const KindProcessListingImage = "process_listing-image"

type ImageSource struct {
	Uploadkey string `json:"upload_key"` //path before processing (Uploaded path)
	ObjectKey string `json:"object_key"` //path after processing (Processed path)
}

type ProcessImagePayload struct {
	ListingId uuid.UUID     `json:"listing_id"`
	Sources   []ImageSource `json:"sources"`
}
