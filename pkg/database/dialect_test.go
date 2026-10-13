package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Text read from book files can carry NUL bytes. The database must store the
// text without them rather than fail the write.
func TestNew_StripsNULFromStrings(t *testing.T) {
	t.Parallel()

	db, err := New(newTestConfig(t))
	require.NoError(t, err)
	defer db.Close()

	var got string
	err = db.NewSelect().ColumnExpr("?", "Track 1\x00").Scan(t.Context(), &got)
	require.NoError(t, err)
	assert.Equal(t, "Track 1", got)
}
