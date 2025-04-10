package errcodes

import (
	"fmt"
	"net/http"
)

type Error struct {
	HTTPCode int
	Message  string
	Code     string
}

func (err *Error) Error() string {
	return err.Message
}

func (err *Error) As(target interface{}) bool {
	te, ok := target.(*Error)
	if !ok {
		return false
	}
	te.HTTPCode = err.HTTPCode
	te.Message = err.Message
	te.Code = err.Code
	return true
}

func (err *Error) Is(target error) bool {
	te, ok := target.(*Error)
	if !ok {
		return false
	}
	return te.HTTPCode == err.HTTPCode &&
		te.Message == err.Message &&
		te.Code == err.Code
}

// Forbidden returns a 403 error with a message indicating the action is
// forbidden.
func Forbidden(action string) error {
	return &Error{
		http.StatusForbidden,
		action + " is not allowed.",
		"forbidden",
	}
}

// NotFound returns a 404 error with a message indicating the given resource.
func NotFound(resource string) error {
	return &Error{
		http.StatusNotFound,
		resource + " not found.",
		"not_found",
	}
}

func UnsupportedMediaType() error {
	return &Error{
		http.StatusUnsupportedMediaType,
		"Unsupported Media Type",
		"unsupported_media_type",
	}
}

func UnknownParameter(param string) error {
	return &Error{
		http.StatusUnprocessableEntity,
		fmt.Sprintf("Unknown Parameter %q", param),
		"unknown_parameter",
	}
}

func ValidationTypeError(msg string) error {
	return &Error{
		http.StatusUnprocessableEntity,
		msg,
		"validation_type_error",
	}
}

func ValidationError(msg string) error {
	return &Error{
		http.StatusUnprocessableEntity,
		msg,
		"validation_error",
	}
}

func MalformedPayload() error {
	return &Error{
		http.StatusBadRequest,
		"Malformed Payload",
		"malformed_payload",
	}
}

func EmptyRequestBody() error {
	return &Error{
		http.StatusBadRequest,
		"Request body can't be empty.",
		"empty_request_body",
	}
}
