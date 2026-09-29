package database

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsUniqueViolation(t *testing.T) {
	t.Parallel()

	assert.True(t, IsUniqueViolation(errors.New("constraint failed: UNIQUE constraint failed: genres.name, genres.library_id (2067)")))
	assert.False(t, IsUniqueViolation(errors.New("constraint failed: FOREIGN KEY constraint failed (787)")))
	assert.False(t, IsUniqueViolation(nil))
}

func TestRetrieveOnUniqueViolation(t *testing.T) {
	t.Parallel()

	created := "created"
	existing := "existing"
	retrieved := 0
	retrieve := func() (*string, error) {
		retrieved++
		return &existing, nil
	}

	got, err := RetrieveOnUniqueViolation(&created, nil, retrieve)
	require.NoError(t, err)
	assert.Equal(t, "created", *got)
	assert.Equal(t, 0, retrieved)

	got, err = RetrieveOnUniqueViolation(&created, errors.New("UNIQUE constraint failed: genres.name"), retrieve)
	require.NoError(t, err)
	assert.Equal(t, "existing", *got)
	assert.Equal(t, 1, retrieved)

	insertErr := errors.New("disk I/O error")
	got, err = RetrieveOnUniqueViolation(&created, insertErr, retrieve)
	require.ErrorIs(t, err, insertErr)
	assert.Nil(t, got)
	assert.Equal(t, 1, retrieved)
}
