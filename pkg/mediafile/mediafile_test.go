package mediafile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsedIdentifier(t *testing.T) {
	id := ParsedIdentifier{
		Type:  "isbn_13",
		Value: "9780316769488",
	}
	assert.Equal(t, "isbn_13", id.Type)
	assert.Equal(t, "9780316769488", id.Value)
}

func TestParsedMetadataIdentifiers(t *testing.T) {
	m := ParsedMetadata{
		Title: "Test Book",
		Identifiers: []ParsedIdentifier{
			{Type: "isbn_13", Value: "9780316769488"},
			{Type: "asin", Value: "B08N5WRWNW"},
		},
	}
	assert.Len(t, m.Identifiers, 2)
	assert.Equal(t, "isbn_13", m.Identifiers[0].Type)
}
