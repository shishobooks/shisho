package identifiers

import (
	"net/http"
	"testing"

	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDuplicateTypeError(t *testing.T) {
	t.Parallel()
	var codeErr *errcodes.Error
	require.ErrorAs(t, DuplicateTypeError("isbn_13"), &codeErr)
	assert.Equal(t, http.StatusUnprocessableEntity, codeErr.HTTPCode)
	assert.Equal(t, "validation_error", codeErr.Code)
	assert.Equal(t, "duplicate identifier type: isbn_13", codeErr.Message)
}
