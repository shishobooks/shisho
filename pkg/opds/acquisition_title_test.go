package opds

import (
	"encoding/xml"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func acquisitionLinks(entry Entry) []Link {
	var links []Link
	for _, link := range entry.Links {
		if link.Rel == RelAcquisition {
			links = append(links, link)
		}
	}
	return links
}

func TestBookToEntry_AcquisitionLinkTitlesTellFilesApart(t *testing.T) {
	t.Parallel()

	first, second := "Illustrated Edition", "Anniversary Edition"
	book := &models.Book{
		ID:    1,
		Title: "Harry Potter and the Sorcerer's Stone",
		Files: []*models.File{
			{ID: 1, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Name: &first},
			{ID: 2, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Name: &second},
		},
	}

	entry := (&Service{}).bookToEntryWithKepub("http://example.com/opds/v1", book, "", nil, false)

	links := acquisitionLinks(entry)
	require.Len(t, links, 2)
	assert.Equal(t, "EPUB · Illustrated Edition", links[0].Title)
	assert.Equal(t, "EPUB · Anniversary Edition", links[1].Title)
}

func TestBookToEntry_AcquisitionLinkTitleFallsBackToBookTitle(t *testing.T) {
	t.Parallel()

	blank := "   "
	book := &models.Book{
		ID:    1,
		Title: "Harry Potter and the Sorcerer's Stone",
		Files: []*models.File{
			{ID: 1, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain},
			{ID: 2, FileType: models.FileTypePDF, FileRole: models.FileRoleMain, Name: &blank},
		},
	}

	entry := (&Service{}).bookToEntryWithKepub("http://example.com/opds/v1", book, "", nil, false)

	links := acquisitionLinks(entry)
	require.Len(t, links, 2)
	assert.Equal(t, "EPUB · Harry Potter and the Sorcerer's Stone", links[0].Title)
	assert.Equal(t, "PDF · Harry Potter and the Sorcerer's Stone", links[1].Title)
}

func TestBookToEntry_KepubAcquisitionLinkTitlesSayKePub(t *testing.T) {
	t.Parallel()

	name := "Deluxe"
	book := &models.Book{
		ID:    1,
		Title: "Saga",
		Files: []*models.File{
			{ID: 1, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Name: &name},
			{ID: 2, FileType: models.FileTypeCBZ, FileRole: models.FileRoleMain},
			{ID: 3, FileType: models.FileTypePDF, FileRole: models.FileRoleMain},
		},
	}

	entry := (&Service{}).bookToEntryWithKepub("http://example.com/opds/v1/kepub", book, "", nil, true)

	links := acquisitionLinks(entry)
	require.Len(t, links, 3)
	assert.Equal(t, "KePub · Deluxe", links[0].Title)
	assert.Equal(t, "KePub · Saga", links[1].Title)
	// A PDF is served as itself in a KePub feed, so its label says so.
	assert.Equal(t, "PDF · Saga", links[2].Title)
}

func TestBookToEntry_AcquisitionLinkTitleIsXMLEscaped(t *testing.T) {
	t.Parallel()

	name := `Tom & Jerry's "Best" <Edition>`
	book := &models.Book{
		ID:    1,
		Title: "Cartoons",
		Files: []*models.File{
			{ID: 1, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Name: &name},
		},
	}

	entry := (&Service{}).bookToEntryWithKepub("http://example.com/opds/v1", book, "", nil, false)

	out, err := xml.Marshal(entry)
	require.NoError(t, err)
	assert.Contains(t, string(out), `title="EPUB · Tom &amp; Jerry&#39;s &#34;Best&#34; &lt;Edition&gt;"`)

	var decoded Entry
	require.NoError(t, xml.Unmarshal(out, &decoded))
	links := acquisitionLinks(decoded)
	require.Len(t, links, 1)
	assert.Equal(t, "EPUB · "+name, links[0].Title)
}
