package errcodes

import (
	"fmt"
	"net/http"
	"strings"
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

// Forbidden returns a 403 error with the given message. Like the other
// constructors it uses the message verbatim, so callers pass a full sentence
// such as "You don't have permission to write jobs".
func Forbidden(msg string) error {
	return &Error{
		http.StatusForbidden,
		msg,
		"forbidden",
	}
}

// LibraryAccessDenied returns the 403 for a user who cannot access the
// library a request reaches, whether through a route param or a loaded entity.
func LibraryAccessDenied() error {
	return Forbidden("You don't have access to this library")
}

// Permission names one resource and operation, such as books and read, for a
// permission denial.
type Permission struct {
	Resource  string
	Operation string
}

// PermissionDenied returns the 403 for a user whose role lacks the operation
// on the resource, such as "You don't have permission to read books".
func PermissionDenied(resource, operation string) error {
	return AnyPermissionDenied(Permission{Resource: resource, Operation: operation})
}

// AnyPermissionDenied returns the 403 for a user who holds none of the
// permissions, listing them as alternatives: "You don't have permission to
// read shares, write shares, or read config".
func AnyPermissionDenied(permissions ...Permission) error {
	names := make([]string, len(permissions))
	for i, p := range permissions {
		names[i] = p.Operation + " " + p.Resource
	}
	var listed string
	switch len(names) {
	case 0:
	case 1, 2:
		listed = strings.Join(names, " or ")
	default:
		listed = strings.Join(names[:len(names)-1], ", ") + ", or " + names[len(names)-1]
	}
	return Forbidden("You don't have permission to " + listed)
}

// DemoMode returns a 403 error for actions disabled in Demo Mode.
func DemoMode() error {
	return &Error{
		http.StatusForbidden,
		"This action is unavailable in the demo.",
		"demo_mode",
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

// InvalidState returns the 422 for a well-formed request that the target's
// current state cannot honor, such as downloading a job that has not
// finished or preferring the cover of a file that has none. The distinct
// code separates it from a rejected payload value (validation_error).
func InvalidState(msg string) error {
	return &Error{
		http.StatusUnprocessableEntity,
		msg,
		"invalid_state",
	}
}

// UpstreamError returns the 502 for a request that failed because an
// upstream server it depends on, such as a plugin download host or
// repository, did not answer or answered with an error. The code matches
// the one the audnexus routes send for the same condition.
func UpstreamError(msg string) error {
	return &Error{
		http.StatusBadGateway,
		msg,
		"upstream_error",
	}
}

// PluginLoadFailure returns a 422 error for a plugin that failed to load at
// runtime (e.g., malformed manifest, incompatible host version). The request
// itself was well-formed — the side effect failed.
func PluginLoadFailure(msg string) error {
	return &Error{
		http.StatusUnprocessableEntity,
		msg,
		"plugin_load_failure",
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

// Unauthorized returns a 401 error with the given message.
func Unauthorized(msg string) error {
	return &Error{
		http.StatusUnauthorized,
		msg,
		"unauthorized",
	}
}

// AuthenticationRequired returns the 401 for a request that reaches an
// authenticated route or handler without an authenticated user.
func AuthenticationRequired() error {
	return &Error{
		http.StatusUnauthorized,
		"Authentication required",
		"unauthorized",
	}
}

// InvalidSession returns the 401 for a session token that does not validate,
// because it is malformed, forged, or expired.
func InvalidSession() error {
	return Unauthorized("Invalid or expired token")
}

// UserInactive returns the 401 for an authenticated identity (a session, or
// an API key's owner) whose user no longer exists or has been deactivated.
func UserInactive() error {
	return &Error{
		http.StatusUnauthorized,
		"User not found or inactive",
		"unauthorized",
	}
}

// Conflict returns a 409 error with the given message.
func Conflict(msg string) error {
	return &Error{
		http.StatusConflict,
		msg,
		"conflict",
	}
}

// PasswordResetRequired returns a 403 error indicating the user must reset
// their password before continuing.
func PasswordResetRequired() error {
	return &Error{
		http.StatusForbidden,
		"You must reset your password before accessing this resource.",
		"password_reset_required",
	}
}
