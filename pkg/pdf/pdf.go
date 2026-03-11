package pdf

import (
	stdlog "log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
)

// pdfcpuInit ensures pdfcpu's global configuration is initialized exactly once.
// pdfcpu's NewDefaultConfiguration() writes global state (config file, font cache)
// that is not thread-safe, so we initialize it before any concurrent access.
var pdfcpuInit sync.Once

// Parse reads metadata from a PDF file and returns it in the
// mediafile.ParsedMetadata format for compatibility with the existing scanner.
func Parse(path string) (*mediafile.ParsedMetadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer f.Close()

	// Ensure pdfcpu global state is initialized before creating a configuration.
	// This avoids data races when Parse is called concurrently.
	pdfcpuInit.Do(func() { model.NewDefaultConfiguration() })

	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed

	ctx, err := api.ReadAndValidate(f, conf)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read PDF")
	}

	xrt := ctx.XRefTable

	// Extract title
	title := strings.TrimSpace(xrt.Title)

	// Extract authors (split on comma, ampersand, or semicolon)
	var authors []mediafile.ParsedAuthor
	authorStr := strings.TrimSpace(xrt.Author)
	if authorStr != "" {
		authors = splitAuthors(authorStr)
	}

	// Extract description from Subject
	description := strings.TrimSpace(xrt.Subject)

	// Extract tags from Keywords
	var tags []string
	keywords := strings.TrimSpace(xrt.Keywords)
	if keywords != "" {
		tags = splitKeywords(keywords)
	}

	// Extract release date from CreationDate
	var releaseDate *time.Time
	if xrt.CreationDate != "" {
		if t, ok := parsePDFDate(xrt.CreationDate); ok {
			releaseDate = &t
		}
	}

	// Extract page count
	var pageCount *int
	if xrt.PageCount > 0 {
		pc := xrt.PageCount
		pageCount = &pc
	}

	// Extract cover image (best-effort: don't fail Parse if cover extraction fails).
	var coverData []byte
	var coverMime string
	if cd, cm, err := extractCover(path); err != nil {
		stdlog.Printf("pdf: cover extraction failed for %s: %v", path, err)
	} else {
		coverData = cd
		coverMime = cm
	}

	return &mediafile.ParsedMetadata{
		Title:         title,
		Authors:       authors,
		Description:   description,
		Tags:          tags,
		ReleaseDate:   releaseDate,
		PageCount:     pageCount,
		CoverData:     coverData,
		CoverMimeType: coverMime,
		DataSource:    models.DataSourcePDFMetadata,
	}, nil
}

// splitAuthors splits an author string on comma, ampersand, or semicolon
// delimiters and returns a slice of ParsedAuthor entries.
func splitAuthors(s string) []mediafile.ParsedAuthor {
	// Split on comma, ampersand, or semicolon
	parts := splitOnDelimiters(s)
	authors := make([]mediafile.ParsedAuthor, 0, len(parts))
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if name != "" {
			authors = append(authors, mediafile.ParsedAuthor{Name: name})
		}
	}
	return authors
}

// splitKeywords splits a keywords string on comma or semicolon delimiters.
func splitKeywords(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';'
	})
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		tag := strings.TrimSpace(p)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

// splitOnDelimiters splits a string on comma, ampersand, or semicolon.
func splitOnDelimiters(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '&' || r == ';'
	})
}

// parsePDFDate attempts to parse a PDF date string using pdfcpu's DateTime parser,
// with fallback formats for non-standard dates.
func parsePDFDate(s string) (time.Time, bool) {
	// Use pdfcpu's built-in date parser which handles the standard PDF date format
	// D:YYYYMMDDHHmmSSOHH'mm' and many variants
	if t, ok := types.DateTime(s, true); ok {
		return t, true
	}

	// Fallback: try common date formats
	fallbackFormats := []string{
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02",
		"2006",
	}
	for _, format := range fallbackFormats {
		if t, err := time.Parse(format, s); err == nil {
			return t, true
		}
	}

	return time.Time{}, false
}
