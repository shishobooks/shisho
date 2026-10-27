package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringList_NilStoresAsEmptyArray(t *testing.T) {
	t.Parallel()
	v, err := StringList(nil).Value()
	require.NoError(t, err)
	assert.Equal(t, "[]", v)
}

func TestStringList_RoundTrip(t *testing.T) {
	t.Parallel()
	v, err := StringList{"Circle", "Magazine"}.Value()
	require.NoError(t, err)

	var l StringList
	require.NoError(t, l.Scan(v))
	assert.Equal(t, StringList{"Circle", "Magazine"}, l)
}

func TestStringList_ScanEmptyIsNonNil(t *testing.T) {
	t.Parallel()
	for _, src := range []any{"[]", []byte("[]"), nil} {
		var l StringList
		require.NoError(t, l.Scan(src))
		assert.NotNil(t, l)
		assert.Empty(t, l)
	}
}
