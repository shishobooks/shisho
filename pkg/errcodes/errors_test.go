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
