// Package mobi reads MOBI and AZW3 files: Mobipocket (MOBI6) books, KF8
// books, and combo files that hold a MOBI6 book followed by a KF8 one. It is
// hand-written so Shisho takes on no GPL code (ADR 0009).
package mobi

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/htmlutil"
	"github.com/shishobooks/shisho/pkg/identifiers"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
)

// ErrDRMProtected is returned by Parse for a file whose text is encrypted.
// Such a file is never imported; a plugin input converter can still produce
// a DRM-free copy beside it.
var ErrDRMProtected = errors.New("file is DRM-protected")

// Parse reads the metadata and cover of a MOBI or AZW3 file. A combo file is
// read from its KF8 half, which carries the richer metadata. Series, tags,
// and author roles are never set: the format has no place for them.
func Parse(path string) (*mediafile.ParsedMetadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	db, err := readPDB(f, stat.Size())
	if err != nil {
		return nil, err
	}

	book, err := readBook(db)
	if err != nil {
		return nil, err
	}

	meta := book.metadata()
	meta.CoverData, meta.CoverMimeType = book.cover(db)
	return meta, nil
}

// book is the header metadata is read from, plus the index the cover offset
// counts from.
type book struct {
	header *header
	// firstImage is the absolute index of the first image record. A combo
	// file's images belong to its MOBI6 book, and the index in its KF8
	// header points at no image, so it comes from the MOBI6 header there.
	firstImage uint32
}

func readBook(db *pdb) (*book, error) {
	rec, err := db.record(0, maxHeaderRecord)
	if err != nil {
		return nil, err
	}
	first, err := parseHeader(rec)
	if err != nil {
		return nil, err
	}
	if first.encrypted {
		return nil, errors.WithStack(ErrDRMProtected)
	}
	if err := first.checkCompression(); err != nil {
		return nil, err
	}

	b := &book{header: first, firstImage: first.firstImage}
	if first.isKF8() {
		return b, nil
	}

	// A combo file names the KF8 header record in its MOBI6 header. If that
	// record holds no KF8 header, the MOBI6 book is all there is to read.
	index, ok := first.number(exthKF8Boundary)
	if !ok || index == 0 || int64(index) >= int64(db.count()) {
		return b, nil
	}
	rec, err = db.record(int(index), maxHeaderRecord)
	if err != nil {
		return nil, err
	}
	kf8, err := parseHeader(rec)
	if err != nil || !kf8.isKF8() {
		return b, nil //nolint:nilerr // a bad boundary leaves the MOBI6 book readable
	}
	if kf8.encrypted {
		return nil, errors.WithStack(ErrDRMProtected)
	}
	if err := kf8.checkCompression(); err != nil {
		return nil, err
	}
	b.header = kf8
	return b, nil
}

func (b *book) metadata() *mediafile.ParsedMetadata {
	h := b.header

	title := h.text(exthUpdatedTitle)
	if title == "" {
		title = h.fullName
	}

	publisher := h.text(exthImprint)
	if publisher == "" {
		publisher = h.text(exthPublisher)
	}

	var language *string
	if lang := h.text(exthLanguage); lang != "" {
		language = mediafile.NormalizeLanguage(lang)
	}

	return &mediafile.ParsedMetadata{
		Title:       title,
		Authors:     authors(h.texts(exthAuthor)),
		Description: htmlutil.StripTags(h.text(exthDescription)),
		Publisher:   publisher,
		ReleaseDate: parseDate(h.text(exthPublishingDate)),
		Language:    language,
		Identifiers: parsedIdentifiers(h),
		Genres:      genres(h.texts(exthSubject)),
		DataSource:  models.DataSourceMOBIMetadata,
	}
}

// authors returns the author names in order, without duplicates, turning a
// "Last, First" name (as Calibre sometimes writes them) into "First Last".
func authors(values []string) []mediafile.ParsedAuthor {
	var result []mediafile.ParsedAuthor
	seen := make(map[string]struct{})
	for _, value := range values {
		name := flipName(value)
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, mediafile.ParsedAuthor{Name: name})
	}
	return result
}

// nameSuffixes follow a comma in a "First Last, Jr." name, which is not a
// "Last, First" name.
var nameSuffixes = map[string]struct{}{
	"jr": {}, "jr.": {}, "sr": {}, "sr.": {}, "ii": {}, "iii": {}, "iv": {},
	"phd": {}, "ph.d.": {}, "md": {}, "m.d.": {},
}

func flipName(name string) string {
	parts := strings.Split(name, ",")
	if len(parts) != 2 {
		return name
	}
	last, first := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if last == "" || first == "" {
		return name
	}
	if _, suffix := nameSuffixes[strings.ToLower(first)]; suffix {
		return name
	}
	return first + " " + last
}

// genres splits each subject record on ";" and drops empty and repeated
// entries.
func genres(values []string) []string {
	var result []string
	seen := make(map[string]struct{})
	for _, value := range values {
		for _, genre := range strings.Split(value, ";") {
			genre = strings.TrimSpace(genre)
			if genre == "" {
				continue
			}
			if _, dup := seen[genre]; dup {
				continue
			}
			seen[genre] = struct{}{}
			result = append(result, genre)
		}
	}
	return result
}

// parsedIdentifiers reads ISBNs from the ISBN record and from a source
// record holding "urn:isbn:...", and the ASIN from the ASIN record. Calibre
// writes the book's UUID into the ASIN record, so only a value shaped like an
// ASIN counts.
func parsedIdentifiers(h *header) []mediafile.ParsedIdentifier {
	var result []mediafile.ParsedIdentifier
	seen := make(map[string]struct{})
	add := func(idType identifiers.Type, value string) {
		if idType == identifiers.TypeUnknown {
			return
		}
		key := identifiers.Key(string(idType), value)
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		result = append(result, mediafile.ParsedIdentifier{Type: string(idType), Value: value})
	}

	for _, isbn := range h.texts(exthISBN) {
		add(identifiers.DetectType(isbn, "ISBN"), isbn)
	}
	for _, source := range h.texts(exthSource) {
		if len(source) > len("urn:isbn:") && strings.EqualFold(source[:len("urn:isbn:")], "urn:isbn:") {
			isbn := strings.TrimSpace(source[len("urn:isbn:"):])
			add(identifiers.DetectType(isbn, "ISBN"), isbn)
		}
	}
	for _, asin := range h.texts(exthASIN) {
		if identifiers.DetectType(asin, "") == identifiers.TypeASIN {
			add(identifiers.TypeASIN, asin)
		}
	}
	return result
}

var dateFormats = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02",
	"2006-01",
	"2006",
}

func parseDate(value string) *time.Time {
	for _, format := range dateFormats {
		if t, err := time.Parse(format, value); err == nil {
			return &t
		}
	}
	return nil
}

// cover returns the cover image and its mime type, or nothing when the file
// names no cover or the record it names is not an image. The cover-offset
// record counts from the first image; a KF8 file may name it only through a
// kindle:embed URI, whose base-32 number counts from 1.
func (b *book) cover(db *pdb) ([]byte, string) {
	if b.firstImage == nullIndex {
		return nil, ""
	}
	offset, ok := b.header.number(exthCoverOffset)
	if !ok || offset == nullIndex {
		uri := b.header.text(exthKF8CoverURI)
		n, err := strconv.ParseUint(strings.TrimPrefix(uri, "kindle:embed:"), 32, 32)
		if !strings.HasPrefix(uri, "kindle:embed:") || err != nil || n == 0 {
			return nil, ""
		}
		offset = uint32(n - 1) //nolint:gosec // ParseUint with bitSize 32 keeps n within uint32
	}

	index := int64(b.firstImage) + int64(offset)
	if index >= int64(db.count()) {
		return nil, ""
	}
	data, err := db.record(int(index), maxImageRecord)
	if err != nil {
		return nil, ""
	}
	mimeType := imageMimeType(data)
	if mimeType == "" {
		return nil, ""
	}
	return data, mimeType
}

func imageMimeType(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(data, []byte("GIF8")):
		return "image/gif"
	case bytes.HasPrefix(data, []byte("BM")):
		return "image/bmp"
	}
	return ""
}
