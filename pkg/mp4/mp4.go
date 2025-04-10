package mp4

import (
	"os"
	"strings"

	"github.com/dhowden/tag"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/mediafile"
)

func Parse(path string) (*mediafile.ParsedMetadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	authors := make([]string, 0)
	if m.Artist() != "" {
		artists := strings.Split(m.Artist(), ",")
		for _, artist := range artists {
			authors = append(authors, strings.TrimSpace(artist))
		}
	}

	coverMimeType := ""
	var coverData []byte

	raw := m.Raw()
	if p, ok := raw["covr"]; ok {
		if picture, ok := p.(*tag.Picture); ok {
			coverMimeType = picture.MIMEType
			coverData = picture.Data
		}
	}

	return &mediafile.ParsedMetadata{
		Title:         m.Title(),
		Authors:       authors,
		CoverMimeType: coverMimeType,
		CoverData:     coverData,
		DataSource:    books.DataSourceM4BMetadata,
	}, nil
}
