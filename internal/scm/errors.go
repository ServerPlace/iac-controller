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
// ADO policy/build-validation errors surface as 400 or 403 but are temporary
// (pipeline still running, approvals pending). Use ADO TF-error-codes for
// precise permanent classification instead of relying solely on HTTP status.
// retryableHTTPCode returns true for codes that are retryable by HTTP status alone.
// For finer-grained classification (e.g. specific 403 messages), callers should
// override Retryable after construction using an explicit whitelist.
func retryableHTTPCode(code int) bool {
	switch {
	case code == 0:
		return true // network/transport error
	case code >= 500:
		return true // SCM unavailable
	default:
		return false // not retryable by default — caller whitelists specific cases
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
