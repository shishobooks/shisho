package mp4

import (
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/dhowden/tag"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
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

	narrators := make([]string, 0)
	if m.Composer() != "" {
		composers := strings.Split(m.Composer(), ",")
		for _, composer := range composers {
			narrators = append(narrators, strings.TrimSpace(composer))
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

	// Parse series information from the Grouping tag
	var series string
	var seriesNumber *float64
	if grouping := m.Album(); grouping != "" {
		// Try to extract from Album tag which often contains grouping info
		if parsed := parseSeriesFromGrouping(grouping); parsed.series != "" {
			series = parsed.series
			seriesNumber = parsed.number
		}
	}

	return &mediafile.ParsedMetadata{
		Title:         m.Title(),
		Authors:       authors,
		Narrators:     narrators,
		Series:        series,
		SeriesNumber:  seriesNumber,
		CoverMimeType: coverMimeType,
		CoverData:     coverData,
		DataSource:    models.DataSourceM4BMetadata,
	}, nil
}

type seriesInfo struct {
	series string
	number *float64
}

func parseSeriesFromGrouping(grouping string) seriesInfo {
	// Handle patterns like "Dungeon Crawler Carl #7"
	re := regexp.MustCompile(`^(.+?)\s*#(\d+(?:\.\d+)?)$`)
	if matches := re.FindStringSubmatch(grouping); len(matches) == 3 {
		seriesName := strings.TrimSpace(matches[1])
		if num, err := strconv.ParseFloat(matches[2], 64); err == nil {
			return seriesInfo{series: seriesName, number: &num}
		}
		return seriesInfo{series: seriesName, number: nil}
	}

	// If no pattern matches, return empty
	return seriesInfo{}
}
