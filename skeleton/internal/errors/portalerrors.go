package portalerrors

import "errors"

var (
	ErrTimeout = errors.New("portal timeout")
	ErrCancelled = errors.New("portal cancelled cancelled")
	ErrInvalidResponse = errors.New("invalid portal response")
	ErrNoStreams = errors.New("no streams returned")
)