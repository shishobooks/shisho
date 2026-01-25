package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileIdentifierFields(t *testing.T) {
	t.Parallel()
	fi := FileIdentifier{
		FileID: 1,
		Type:   "isbn_13",
		Value:  "9780316769488",
		Source: DataSourceEPUBMetadata,
	}
	assert.Equal(t, 1, fi.FileID)
	assert.Equal(t, "isbn_13", fi.Type)
	assert.Equal(t, "9780316769488", fi.Value)
	assert.Equal(t, DataSourceEPUBMetadata, fi.Source)
}
