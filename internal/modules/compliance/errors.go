package compliance

import "errors"

var (
	// ErrRequestNotFound is returned when a data subject request is not found.
	ErrRequestNotFound = errors.New("compliance: request not found")

	// ErrRequestAlreadyExists is returned when a request already exists.
	ErrRequestAlreadyExists = errors.New("compliance: request already exists")

	// ErrExportJobNotFound is returned when an export job is not found.
	ErrExportJobNotFound = errors.New("compliance: export job not found")

	// ErrExportJobAlreadyExists is returned when an export job already exists.
	ErrExportJobAlreadyExists = errors.New("compliance: export job already exists")

	// ErrDeletionJobNotFound is returned when a deletion job is not found.
	ErrDeletionJobNotFound = errors.New("compliance: deletion job not found")

	// ErrDeletionJobAlreadyExists is returned when a deletion job already exists.
	ErrDeletionJobAlreadyExists = errors.New("compliance: deletion job already exists")

	// ErrInvalidRequestType is returned when an invalid request type is specified.
	ErrInvalidRequestType = errors.New("compliance: invalid request type")

	// ErrInvalidExportFormat is returned when an invalid export format is specified.
	ErrInvalidExportFormat = errors.New("compliance: invalid export format")

	// ErrExportInProgress is returned when an export is already in progress.
	ErrExportInProgress = errors.New("compliance: export already in progress")

	// ErrDeletionInProgress is returned when a deletion is already in progress.
	ErrDeletionInProgress = errors.New("compliance: deletion already in progress")

	// ErrDeletionNotConfirmed is returned when deletion is not confirmed.
	ErrDeletionNotConfirmed = errors.New("compliance: deletion must be confirmed")

	// ErrUserNotFound is returned when a user is not found.
	ErrUserNotFound = errors.New("compliance: user not found")

	// ErrExportFailed is returned when export generation fails.
	ErrExportFailed = errors.New("compliance: export generation failed")

	// ErrDeletionFailed is returned when data deletion fails.
	ErrDeletionFailed = errors.New("compliance: data deletion failed")
)
