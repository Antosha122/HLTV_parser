package storage

import "errors"

// Domain errors for the storage layer. Use errors.Is to check for them so
// callers don't have to parse SQL error strings.
var (
	// ErrNotFound is returned when a single-row lookup (GetTeam, GetMatch, ...)
	// doesn't find an entry.
	ErrNotFound = errors.New("storage: not found")

	// ErrConflict is returned when an upsert conflicts in a way the caller may
	// want to handle (e.g. a constraint violation beyond idempotent ON CONFLICT).
	ErrConflict = errors.New("storage: conflict")
)
