package identity

import "errors"

var (
	// ErrUserNotFound indicates the requested user could not be located.
	ErrUserNotFound = errors.New("identity: user not found")

	// ErrInvalidCredentials indicates supplied credentials are invalid.
	ErrInvalidCredentials = errors.New("identity: invalid credentials")

	// ErrRoleNotPermitted indicates the user cannot authenticate with requested role.
	ErrRoleNotPermitted = errors.New("identity: role not permitted for user")

	// ErrSessionNotFound indicates the requested session was not found.
	ErrSessionNotFound = errors.New("identity: session not found")

	// ErrStateMismatch indicates an OAuth state token mismatch.
	ErrStateMismatch = errors.New("identity: oauth state mismatch")

	// ErrTwoFactorConflict indicates conflicting toggles were supplied.
	ErrTwoFactorConflict = errors.New("identity: two-factor conflict")

	// ErrInvalidPermission indicates an unknown permission has been requested.
	ErrInvalidPermission = errors.New("identity: invalid permission")
)
