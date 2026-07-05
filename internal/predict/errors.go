package predict

import "errors"

// Domain errors for the prediction engine. Map these to HTTP codes in the API
// layer; do not string-match error messages.
var (
	// ErrInsufficientData is returned when there isn't enough historical data to
	// build a reliable prediction (e.g. missing teams, no matches, no form).
	ErrInsufficientData = errors.New("predict: insufficient data")

	// ErrInvalidArgument is returned for bad input (e.g. invalid team ids).
	ErrInvalidArgument = errors.New("predict: invalid argument")
)
