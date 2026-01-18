package superset

import "errors"

var (
	// ErrDashboardNotFound is returned when a dashboard is not found.
	ErrDashboardNotFound = errors.New("superset: dashboard not found")

	// ErrAuthenticationFailed is returned when authentication fails.
	ErrAuthenticationFailed = errors.New("superset: authentication failed")

	// ErrGuestTokenFailed is returned when guest token generation fails.
	ErrGuestTokenFailed = errors.New("superset: guest token generation failed")

	// ErrServiceUnavailable is returned when Superset is unavailable.
	ErrServiceUnavailable = errors.New("superset: service unavailable")
)
