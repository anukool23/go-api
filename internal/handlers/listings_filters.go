package handlers

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	statusReady    = "ready"
	statusPending  = "processing"
	statusRejected = "rejected"

	defaultListingStatus = "ready"

	maxCityLength = 100
)

type listingFilters struct {
	Status   string
	City     string
	MinPrice *int64
	MaxPrice *int64
}

func parseListingFilters(params url.Values) (listingFilters, error) {
	var f listingFilters
	rawStatus := strings.TrimSpace(params.Get("status"))
	if rawStatus == "" {
		rawStatus = defaultListingStatus
	}
	status, ok := canonicalListingStatus(rawStatus)
	if !ok {
		return listingFilters{}, &ValidationError{Field: "status", Message: fmt.Sprintf("must be one of %s, %s, %s", statusReady, statusPending, statusRejected)}
	}
	f.Status = status

	city := strings.TrimSpace(params.Get("city"))
	if len(city) > maxCityLength {
		return listingFilters{}, &ValidationError{Field: "city", Message: fmt.Sprintf("must be at most %d chars", maxCityLength)}
	}
	f.City = city
	minPrice, err := parsePrice(params.Get("min_price"), "min_price")
	if err != nil {
		return listingFilters{}, err
	}
	f.MinPrice = minPrice

	maxPrice, err := parsePrice(params.Get("max_price"), "max_price")
	if err != nil {
		return listingFilters{}, err
	}
	f.MaxPrice = maxPrice

	return f, nil
}

func canonicalListingStatus(rawStatus string) (string, bool) {
	switch rawStatus {
	case statusReady:
		return statusReady, true
	case statusPending:
		return statusPending, true
	case statusRejected:
		return statusRejected, true
	default:
		return "", false

	}
}

func parsePrice(raw, field string) (*int64, error) {
	if raw == "" {
		return nil, nil
	}
	num, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, &ValidationError{Field: field, Message: "must be an integer and not be negative"}
	}
	if num < 0 {
		return nil, &ValidationError{Field: field, Message: "must be an integer and not be negative"}
	}
	return &num, nil
}

// var query = `
// 	SELECT l.id, l.title, l.description, l.price, l.city, l.status, l.created_at, l.user_id, i.id as imageId, i.object_key, i.position
// 	FROM listings l
// 	LEFT JOIN images i ON i.listing_id = l.id
// 	WHERE l.id IN (
// 		SELECT id FROM listings l2 WHERE l2.status = 'ready' AND (l2.created_at, l2.id) < ($1,$2)
// 		ORDER BY l2.created_at DESC, l2.id DESC LIMIT $3
// 	)
// 	ORDER BY l.created_at DESC, l.id DESC, i.position
// 	`

const queryTemplate = `
	SELECT l.id, l.title, l.description, l.price, l.city, l.status, l.created_at, l.user_id, i.id as imageId, i.object_key, i.position
	FROM listings l
	LEFT JOIN images i ON i.listing_id = l.id
	WHERE l.id IN (
		SELECT id FROM listings l2 
		WHERE %s
		ORDER BY l2.created_at DESC, l2.id DESC 
		LIMIT $%d
	)
	ORDER BY l.created_at DESC, l.id DESC, i.position
	`

func buildListingQuery(filters listingFilters, cursor *listingCursor, limit int) (string, []any) {
	where := []string{fmt.Sprintf("l2.status = '%s'", filters.Status)}
	args := []any{}
	if filters.City != "" {
		args = append(args, strings.ToLower(filters.City))
		where = append(where, fmt.Sprintf("lower(l2.city) = $%d", len(args)))
	}
	if filters.MinPrice != nil {
		args = append(args, *filters.MinPrice)
		where = append(where, fmt.Sprintf("l2.price >= $%d", len(args)))
	}
	if filters.MaxPrice != nil {
		args = append(args, *filters.MaxPrice)
		where = append(where, fmt.Sprintf("l2.price <= $%d", len(args)))
	}
	if cursor != nil {
		args = append(args, cursor.CreatedAt, cursor.ID)
		where = append(where, fmt.Sprintf("(l2.created_at, l2.id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	args = append(args, limit)

	query := fmt.Sprintf(queryTemplate, strings.Join(where, "\n\t\t AND "), len(args))
	fmt.Print(query)

	return query, args

}
