package errcodes

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForbidden(t *testing.T) {
	t.Parallel()

	err := Forbidden("You don't have permission to write jobs")

	var codeErr *Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, http.StatusForbidden, codeErr.HTTPCode)
	assert.Equal(t, "forbidden", codeErr.Code)
	// The message is used verbatim; callers write full sentences.
	assert.Equal(t, "You don't have permission to write jobs", err.Error())
}

// Each shared constructor renders one fixed status, wire code, and message,
// so every call site that reports the same condition reads the same.
func TestSharedConstructors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     error
		status  int
		code    string
		message string
	}{
		{"InvalidSession", InvalidSession(), http.StatusUnauthorized, "unauthorized", "Invalid or expired token"},
		{"AuthenticationRequired", AuthenticationRequired(), http.StatusUnauthorized, "unauthorized", "Authentication required"},
		{"UserInactive", UserInactive(), http.StatusUnauthorized, "unauthorized", "User not found or inactive"},
		{"LibraryAccessDenied", LibraryAccessDenied(), http.StatusForbidden, "forbidden", "You don't have access to this library"},
		{"PermissionDenied", PermissionDenied("books", "read"), http.StatusForbidden, "forbidden", "You don't have permission to read books"},
		{
			"AnyPermissionDenied with one permission",
			AnyPermissionDenied(Permission{Resource: "jobs", Operation: "write"}),
			http.StatusForbidden, "forbidden", "You don't have permission to write jobs",
		},
		{
			"AnyPermissionDenied with two permissions",
			AnyPermissionDenied(Permission{Resource: "shares", Operation: "read"}, Permission{Resource: "shares", Operation: "write"}),
			http.StatusForbidden, "forbidden", "You don't have permission to read shares or write shares",
		},
		{
			"AnyPermissionDenied with three permissions",
			AnyPermissionDenied(
				Permission{Resource: "shares", Operation: "read"},
				Permission{Resource: "shares", Operation: "write"},
				Permission{Resource: "config", Operation: "read"},
			),
			http.StatusForbidden, "forbidden", "You don't have permission to read shares, write shares, or read config",
		},
		{"InvalidState", InvalidState("Job is not completed yet"), http.StatusUnprocessableEntity, "invalid_state", "Job is not completed yet"},
		{"UpstreamError", UpstreamError("Could not download the plugin"), http.StatusBadGateway, "upstream_error", "Could not download the plugin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var codeErr *Error
			require.ErrorAs(t, tt.err, &codeErr)
			assert.Equal(t, tt.status, codeErr.HTTPCode)
			assert.Equal(t, tt.code, codeErr.Code)
			assert.Equal(t, tt.message, codeErr.Message)
		})
	}
}
