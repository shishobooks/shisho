package downloadcache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
)

// Fingerprint represents the metadata that affects file generation.
// Changes to any of these fields should invalidate the cached file.
type Fingerprint struct {
	Title    string              `json:"title"`
	Subtitle *string             `json:"subtitle,omitempty"`
	Authors  []FingerprintAuthor `json:"authors"`
	Series   []FingerprintSeries `json:"series"`
	Cover    *FingerprintCover   `json:"cover,omitempty"`
}

// FingerprintAuthor represents author information for fingerprinting.
type FingerprintAuthor struct {
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

// FingerprintSeries represents series information for fingerprinting.
type FingerprintSeries struct {
	Name      string   `json:"name"`
	Number    *float64 `json:"number,omitempty"`
	SortOrder int      `json:"sort_order"`
}

// FingerprintCover represents cover image information for fingerprinting.
type FingerprintCover struct {
	Path     string    `json:"path"`
	MimeType string    `json:"mime_type"`
	ModTime  time.Time `json:"mod_time"`
}

// ComputeFingerprint creates a fingerprint from a book and file.
func ComputeFingerprint(book *models.Book, file *models.File) (*Fingerprint, error) {
	fp := &Fingerprint{
		Title:    book.Title,
		Subtitle: book.Subtitle,
		Authors:  make([]FingerprintAuthor, 0),
		Series:   make([]FingerprintSeries, 0),
	}

	// Add authors sorted by SortOrder for consistent fingerprinting
	if len(book.Authors) > 0 {
		authors := make([]*models.Author, len(book.Authors))
		copy(authors, book.Authors)
		sort.Slice(authors, func(i, j int) bool {
			return authors[i].SortOrder < authors[j].SortOrder
		})
		for _, a := range authors {
			if a.Person != nil {
				fp.Authors = append(fp.Authors, FingerprintAuthor{
					Name:      a.Person.Name,
					SortOrder: a.SortOrder,
				})
			}
		}
	}

	// Add series sorted by SortOrder for consistent fingerprinting
	if len(book.BookSeries) > 0 {
		series := make([]*models.BookSeries, len(book.BookSeries))
		copy(series, book.BookSeries)
		sort.Slice(series, func(i, j int) bool {
			return series[i].SortOrder < series[j].SortOrder
		})
		for _, s := range series {
			if s.Series != nil {
				fp.Series = append(fp.Series, FingerprintSeries{
					Name:      s.Series.Name,
					Number:    s.SeriesNumber,
					SortOrder: s.SortOrder,
				})
			}
		}
	}

	// Add cover information if present
	if file.CoverImagePath != nil && *file.CoverImagePath != "" {
		coverPath := *file.CoverImagePath
		mimeType := ""
		if file.CoverMimeType != nil {
			mimeType = *file.CoverMimeType
		}

		// Get file modification time for the cover
		var modTime time.Time
		if info, err := os.Stat(coverPath); err == nil {
			modTime = info.ModTime()
		}

		fp.Cover = &FingerprintCover{
			Path:     coverPath,
			MimeType: mimeType,
			ModTime:  modTime,
		}
	}

	return fp, nil
}

// Hash computes a SHA256 hash of the fingerprint.
func (fp *Fingerprint) Hash() (string, error) {
	data, err := json.Marshal(fp)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// Equal compares two fingerprints for equality.
func (fp *Fingerprint) Equal(other *Fingerprint) bool {
	if fp == nil && other == nil {
		return true
	}
	if fp == nil || other == nil {
		return false
	}

	hash1, err1 := fp.Hash()
	hash2, err2 := other.Hash()

	if err1 != nil || err2 != nil {
		return false
	}

	return hash1 == hash2
}
