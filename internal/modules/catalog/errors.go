package catalog

import "errors"

var (
	// Override errors
	ErrOverrideNotFound = errors.New("catalog override not found")
	ErrItemNotFound     = errors.New("catalog item not found")

	// Outlet errors
	ErrOutletNotFound = errors.New("outlet not found")

	// General errors
	ErrInvalidTenant = errors.New("invalid tenant")
	ErrUnauthorized  = errors.New("unauthorized access")
	ErrInternalError = errors.New("internal server error")
)
