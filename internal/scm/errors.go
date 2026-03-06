package scm

import "fmt"

// MergeError wraps an SCM merge failure with an HTTP status code and a retryability flag.
//
// Retryable=true  → transient: network errors (Code 0), 400 (policy/conflict), 5xx
// Retryable=false → permanent: 401 (invalid PAT), 403 (no permission), 404 (not found)
type MergeError struct {
	Err       error
	Code      int // HTTP status code; 0 for network/transport errors
	Retryable bool
}

func (e *MergeError) Error() string {
	return fmt.Sprintf("merge failed (http %d, retryable=%v): %v", e.Code, e.Retryable, e.Err)
}

func (e *MergeError) Unwrap() error { return e.Err }

// retryableHTTPCode returns true for codes that warrant a retry.
func retryableHTTPCode(code int) bool {
	switch {
	case code == 0:
		return true // network/transport error
	case code == 400:
		return true // merge conflict, policy pending, pipeline running
	case code >= 500:
		return true // SCM unavailable
	default:
		return false // 401, 403, 404, etc.
	}
}

// NewMergeError builds a MergeError from a raw error and its HTTP status code.
func NewMergeError(err error, code int) *MergeError {
	return &MergeError{
		Err:       err,
		Code:      code,
		Retryable: retryableHTTPCode(code),
	}
}
