package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJournalRequests_CountsFromTheFreshServersStart(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "server.log")
	before := `{"route":"/mobile/journal","message":"request handled"}` + "\n"
	after := `{"route":"/mobile/changes","message":"request handled"}` + "\n" +
		`{"route":"/mobile/journal","message":"request handled"}` + "\n"
	require.NoError(t, os.WriteFile(path, []byte(before+after), 0o600))

	count, err := journalRequests(path, int64(len(before)))
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	count, err = journalRequests(path, int64(len(before+after)))
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
