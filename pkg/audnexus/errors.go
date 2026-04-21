package audnexus

import "errors"

// ErrorCode is a stable string identifier used by both the service and the
// HTTP handler to map failures to user-facing responses.
type ErrorCode string

const (
	ErrCodeInvalidASIN   ErrorCode = "invalid_asin"
	ErrCodeNotFound      ErrorCode = "not_found"
	ErrCodeTimeout       ErrorCode = "timeout"
	ErrCodeUpstreamError ErrorCode = "upstream_error"
	ErrCodeRateLimited   ErrorCode = "rate_limited"
)

// Error is a typed error carrying an ErrorCode. Callers can use errors.As to
// inspect the code for mapping to HTTP status.
type Error struct {
	Code ErrorCode
	Msg  string
}

func (e *Error) Error() string {
	if e.Msg != "" {
		return string(e.Code) + ": " + e.Msg
	}
	return string(e.Code)
}

func newErr(code ErrorCode, msg string) error {
	return &Error{Code: code, Msg: msg}
}

// AsAudnexusError extracts an *Error if err wraps one, else returns nil.
func AsAudnexusError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}
