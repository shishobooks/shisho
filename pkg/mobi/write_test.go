package mobi

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testImage encodes a noisy w x h image, so its size grows with its area.
func testImage(t *testing.T, w, h int, mimeType string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rng := rand.New(rand.NewPCG(uint64(w), uint64(h))) //nolint:gosec // test noise
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(rng.UintN(256)), uint8(rng.UintN(256)), uint8(rng.UintN(256)), 255})
		}
	}
	var buf bytes.Buffer
	var err error
	if mimeType == "image/png" {
		err = png.Encode(&buf, img)
	} else {
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	}
	require.NoError(t, err)
	return buf.Bytes()
}

// rewriteBuilt builds a file, rewrites it, and returns both byte slices.
func rewriteBuilt(t *testing.T, opts testgen.MOBIOptions, meta *Metadata) ([]byte, []byte) {
	t.Helper()
	src := testgen.BuildMOBI(t, opts)
	var out bytes.Buffer
	require.NoError(t, Rewrite(bytes.NewReader(src), int64(len(src)), &out, meta))
	return src, out.Bytes()
}

func parseBytes(t *testing.T, data []byte) *mediafile.ParsedMetadata {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out.mobi")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	meta, err := Parse(path)
	require.NoError(t, err)
	return meta
}

func openPDB(t *testing.T, data []byte) *pdb {
	t.Helper()
	db, err := readPDB(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	return db
}

func headerAt(t *testing.T, db *pdb, i int) *header {
	t.Helper()
	rec, err := db.record(i, maxHeaderRecord)
	require.NoError(t, err)
	h, err := parseHeader(rec)
	require.NoError(t, err)
	return h
}

func fullMetadata(cover []byte) *Metadata {
	date := time.Date(2024, 5, 6, 0, 0, 0, 0, time.UTC)
	return &Metadata{
		Title:       "A New Title",
		Authors:     []string{"Ann Author", "Bea Writer"},
		Description: "A fresh description.",
		Publisher:   "New House",
		ReleaseDate: &date,
		Language:    "en-US",
		Identifiers: &Identifiers{ISBNs: []string{"9780141036144"}, ASIN: "B00ABCDEFG"},
		Genres:      []string{"Mystery", "Thriller"},
		Cover:       cover,
	}
}

func TestRewrite_RoundTrip(t *testing.T) {
	t.Parallel()

	cases := map[string]testgen.MOBIOptions{
		"MOBI6":          calibreOptions(testgen.MOBIKindMOBI6),
		"AZW3":           calibreOptions(testgen.MOBIKindKF8),
		"combo":          calibreOptions(testgen.MOBIKindCombo),
		"MOBI6 no cover": withoutCover(calibreOptions(testgen.MOBIKindMOBI6)),
		"AZW3 no cover":  withoutCover(calibreOptions(testgen.MOBIKindKF8)),
		"combo no cover": withoutCover(calibreOptions(testgen.MOBIKindCombo)),
	}
	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			before := parseBytes(t, testgen.BuildMOBI(t, opts))
			assert.Equal(t, "The Test Book", before.Title)

			// Larger than the original cover, so a replacement that only fit
			// in place would fail.
			cover := testImage(t, 400, 600, "image/jpeg")
			_, out := rewriteBuilt(t, opts, fullMetadata(cover))
			after := parseBytes(t, out)

			assert.Equal(t, "A New Title", after.Title)
			assert.Equal(t, []mediafile.ParsedAuthor{{Name: "Ann Author"}, {Name: "Bea Writer"}}, after.Authors)
			assert.Equal(t, "A fresh description.", after.Description)
			assert.Equal(t, "New House", after.Publisher)
			require.NotNil(t, after.ReleaseDate)
			assert.Equal(t, time.Date(2024, 5, 6, 0, 0, 0, 0, time.UTC), after.ReleaseDate.UTC())
			require.NotNil(t, after.Language)
			assert.Equal(t, "en-US", *after.Language)
			assert.Equal(t, []mediafile.ParsedIdentifier{
				{Type: "isbn_13", Value: "9780141036144"},
				{Type: "asin", Value: "B00ABCDEFG"},
			}, after.Identifiers)
			assert.Equal(t, []string{"Mystery", "Thriller"}, after.Genres)
			assert.Equal(t, "image/jpeg", after.CoverMimeType)
			assert.Equal(t, cover, after.CoverData)

			// The rewritten file rewrites again, which a broken record table
			// or header would not survive.
			var again bytes.Buffer
			require.NoError(t, Rewrite(bytes.NewReader(out), int64(len(out)), &again, &Metadata{Title: "Third"}))
			third := parseBytes(t, again.Bytes())
			assert.Equal(t, "Third", third.Title)
			assert.Equal(t, cover, third.CoverData)
		})
	}
}

func withoutCover(opts testgen.MOBIOptions) testgen.MOBIOptions {
	opts.HasCover = false
	return opts
}

func TestRewrite_ComboRewritesBothHeaders(t *testing.T) {
	t.Parallel()

	for name, hasCover := range map[string]bool{"with cover": true, "no cover": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			opts := calibreOptions(testgen.MOBIKindCombo)
			opts.MOBI6Title = "Old MOBI6 Title"
			opts.HasCover = hasCover
			src, out := rewriteBuilt(t, opts, fullMetadata(testImage(t, 40, 60, "image/jpeg")))

			db := openPDB(t, out)
			srcDB := openPDB(t, src)
			mobi6 := headerAt(t, db, 0)
			assert.False(t, mobi6.isKF8())
			assert.Equal(t, "A New Title", mobi6.text(exthUpdatedTitle))
			assert.Equal(t, "A New Title", mobi6.fullName)
			assert.Equal(t, []string{"Ann Author", "Bea Writer"}, mobi6.texts(exthAuthor))

			kf8Index, ok := mobi6.number(exthKF8Boundary)
			require.True(t, ok)
			kf8 := headerAt(t, db, int(kf8Index))
			assert.True(t, kf8.isKF8(), "the boundary record still names the KF8 header")
			assert.Equal(t, "A New Title", kf8.text(exthUpdatedTitle))
			assert.Equal(t, "A New Title", kf8.fullName)

			srcMOBI6 := headerAt(t, srcDB, 0)
			srcKF8Index, _ := srcMOBI6.number(exthKF8Boundary)
			if hasCover {
				assert.Equal(t, srcKF8Index, kf8Index)
				assert.Equal(t, srcDB.count(), db.count())
			} else {
				assert.Equal(t, srcKF8Index+1, kf8Index, "the new cover record sits in the MOBI6 half")
				assert.Equal(t, srcDB.count()+1, db.count())
			}
			assertTrailerIndexes(t, db, 0)
			assertTrailerIndexes(t, db, int(kf8Index))
			assertRecordsUnchanged(t, srcDB, db, 1, int(srcKF8Index))
		})
	}
}

// assertTrailerIndexes checks that the FLIS and FCIS indexes (and FDST in a
// KF8 header) of the header at base still name those records.
func assertTrailerIndexes(t *testing.T, db *pdb, base int) {
	t.Helper()
	rec, err := db.record(base, maxHeaderRecord)
	require.NoError(t, err)
	h := headerAt(t, db, base)
	fields := map[int]string{offFCIS: "FCIS", offFLIS: "FLIS"}
	if h.isKF8() {
		fields[offFDST] = "FDST"
	}
	for off, magic := range fields {
		index := int(binary.BigEndian.Uint32(rec[off:]))
		trailer, err := db.record(base+index, maxImageRecord)
		require.NoError(t, err)
		assert.Equal(t, magic, string(trailer[:4]), "header at %d, field %#x", base, off)
	}
}

// assertRecordsUnchanged checks that the text records of the book whose
// header is at base are byte-identical in both files.
func assertRecordsUnchanged(t *testing.T, src, out *pdb, from, to int) {
	t.Helper()
	for i := from; i < to; i++ {
		a, err := src.record(i, maxImageRecord)
		require.NoError(t, err)
		if imageMimeType(a) != "" || isTrailerRecord(a[:min(len(a), 8)], int64(len(a))) {
			break
		}
		b, err := out.record(i, maxImageRecord)
		require.NoError(t, err)
		assert.Equal(t, a, b, "record %d", i)
	}
}

func TestRewrite_NoCoverInsertsImageRecord(t *testing.T) {
	t.Parallel()

	for name, kind := range map[string]testgen.MOBIKind{
		"MOBI6": testgen.MOBIKindMOBI6,
		"AZW3":  testgen.MOBIKindKF8,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			opts := withoutCover(calibreOptions(kind))
			cover := testImage(t, 40, 60, "image/jpeg")
			src, out := rewriteBuilt(t, opts, &Metadata{Cover: cover})
			srcDB, db := openPDB(t, src), openPDB(t, out)
			require.Equal(t, srcDB.count()+1, db.count())

			h := headerAt(t, db, 0)
			require.NotEqual(t, uint32(nullIndex), h.firstImage)
			offset, ok := h.number(exthCoverOffset)
			require.True(t, ok)
			got, err := db.record(int(h.firstImage+offset), maxImageRecord)
			require.NoError(t, err)
			assert.Equal(t, cover, got)
			if h.isKF8() {
				assert.Equal(t, "kindle:embed:0001", h.text(exthKF8CoverURI))
			}
			assertTrailerIndexes(t, db, 0)
			assertRecordsUnchanged(t, srcDB, db, 1, srcDB.count())
		})
	}
}

func TestRewrite_ReplacesThumbnail(t *testing.T) {
	t.Parallel()

	cover := testImage(t, 40, 60, "image/jpeg")
	_, out := rewriteBuilt(t, calibreOptions(testgen.MOBIKindMOBI6), &Metadata{Cover: cover})
	db := openPDB(t, out)
	h := headerAt(t, db, 0)
	thumb, ok := h.number(exthThumbOffset)
	require.True(t, ok)
	got, err := db.record(int(h.firstImage+thumb), maxImageRecord)
	require.NoError(t, err)
	assert.Equal(t, cover, got)
}

func TestRewrite_PNGCoverBecomesJPEG(t *testing.T) {
	t.Parallel()

	// MOBI6 readers cannot show a PNG, and a combo file's images are the
	// MOBI6 book's, so every cover is written as JPEG.
	_, out := rewriteBuilt(t, calibreOptions(testgen.MOBIKindCombo), &Metadata{Cover: testImage(t, 40, 60, "image/png")})
	after := parseBytes(t, out)
	assert.Equal(t, "image/jpeg", after.CoverMimeType)
	_, err := jpeg.Decode(bytes.NewReader(after.CoverData))
	require.NoError(t, err)
}

func TestRewrite_EmptyFieldsKeepFileValues(t *testing.T) {
	t.Parallel()

	_, out := rewriteBuilt(t, calibreOptions(testgen.MOBIKindKF8), &Metadata{})
	after := parseBytes(t, out)
	before := parseBytes(t, testgen.BuildMOBI(t, calibreOptions(testgen.MOBIKindKF8)))
	assert.Equal(t, before, after)
}

func TestRewrite_PublisherReplacesImprint(t *testing.T) {
	t.Parallel()

	opts := calibreOptions(testgen.MOBIKindMOBI6)
	opts.Imprint = "Old Imprint"
	_, out := rewriteBuilt(t, opts, &Metadata{Publisher: "New House"})
	assert.Equal(t, "New House", parseBytes(t, out).Publisher)
}

func TestRewrite_Identifiers(t *testing.T) {
	t.Parallel()

	t.Run("no ASIN keeps Calibre's UUID and drops an old ASIN", func(t *testing.T) {
		t.Parallel()

		opts := calibreOptions(testgen.MOBIKindKF8)
		_, out := rewriteBuilt(t, opts, &Metadata{Identifiers: &Identifiers{ISBNs: []string{"0306406152"}}})
		h := headerAt(t, openPDB(t, out), 0)
		assert.Equal(t, []string{opts.ASIN}, h.texts(exthASIN))
		assert.Equal(t, []string{"0306406152"}, h.texts(exthISBN))

		opts.ASIN = "B00OLDASIN"
		_, out = rewriteBuilt(t, opts, &Metadata{Identifiers: &Identifiers{}})
		assert.Empty(t, parseBytes(t, out).Identifiers)
	})

	t.Run("an ISBN in the source record is dropped with the identifiers", func(t *testing.T) {
		t.Parallel()

		opts := calibreOptions(testgen.MOBIKindKF8)
		opts.ISBN = ""
		opts.Source = "urn:isbn:9780306406157"
		_, out := rewriteBuilt(t, opts, &Metadata{Identifiers: &Identifiers{ASIN: "B00ABCDEFG"}})
		assert.Equal(t, []mediafile.ParsedIdentifier{{Type: "asin", Value: "B00ABCDEFG"}}, parseBytes(t, out).Identifiers)
	})
}

func TestRewrite_CP1252(t *testing.T) {
	t.Parallel()

	opts := calibreOptions(testgen.MOBIKindMOBI6)
	opts.CP1252 = true
	_, out := rewriteBuilt(t, opts, &Metadata{Title: "Café Crème", Authors: []string{"Zoë Ünder", "李 白"}})
	after := parseBytes(t, out)
	assert.Equal(t, "Café Crème", after.Title)
	assert.Equal(t, []mediafile.ParsedAuthor{{Name: "Zoë Ünder"}, {Name: "? ?"}}, after.Authors,
		"text the file's encoding cannot hold is replaced, not dropped")
}

func TestRewrite_DRMProtected(t *testing.T) {
	t.Parallel()

	opts := calibreOptions(testgen.MOBIKindKF8)
	opts.Encrypted = true
	src := testgen.BuildMOBI(t, opts)
	err := Rewrite(bytes.NewReader(src), int64(len(src)), &bytes.Buffer{}, &Metadata{Title: "x"})
	assert.True(t, errors.Is(err, ErrDRMProtected))
}

func TestRewrite_NotMOBI(t *testing.T) {
	t.Parallel()

	src := []byte("not a palm database at all, just some text that is long enough to read a header from")
	err := Rewrite(bytes.NewReader(src), int64(len(src)), &bytes.Buffer{}, &Metadata{})
	assert.True(t, errors.Is(err, errNotMOBI))
}

// Calibre writes a book with no images with a first-image index past the end
// of the book (the record count, in a combo file the whole file's), not the
// null index.
func TestRewrite_NoImagesWithFirstImagePastEnd(t *testing.T) {
	t.Parallel()

	for name, kind := range map[string]testgen.MOBIKind{
		"MOBI6": testgen.MOBIKindMOBI6,
		"AZW3":  testgen.MOBIKindKF8,
		"combo": testgen.MOBIKindCombo,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			src := testgen.BuildMOBI(t, withoutCover(calibreOptions(kind)))
			srcDB := openPDB(t, src)
			binary.BigEndian.PutUint32(src[srcDB.offsets[0]+offFirstImage:], uint32(srcDB.count())) //nolint:gosec // small test file

			cover := testImage(t, 40, 60, "image/jpeg")
			var out bytes.Buffer
			require.NoError(t, Rewrite(bytes.NewReader(src), int64(len(src)), &out, &Metadata{Title: "New", Cover: cover}))

			after := parseBytes(t, out.Bytes())
			assert.Equal(t, "New", after.Title)
			assert.Equal(t, cover, after.CoverData)

			db := openPDB(t, out.Bytes())
			last, err := db.record(db.count()-1, maxImageRecord)
			require.NoError(t, err)
			assert.Equal(t, eofRecord, last, "the cover goes before the end-of-file record")
			h := headerAt(t, db, 0)
			assertTrailerIndexes(t, db, 0)
			if index, ok := h.number(exthKF8Boundary); ok {
				boundary, err := db.record(int(index)-1, maxImageRecord)
				require.NoError(t, err)
				assert.Equal(t, "BOUNDARY", string(boundary))
				assert.True(t, headerAt(t, db, int(index)).isKF8())
				assertTrailerIndexes(t, db, int(index))
			}
		})
	}
}

func TestRewrite_ShiftsSRCSIndex(t *testing.T) {
	t.Parallel()

	src := testgen.BuildMOBI(t, withoutCover(calibreOptions(testgen.MOBIKindMOBI6)))
	srcDB := openPDB(t, src)
	rec0 := src[srcDB.offsets[0]:]
	// Point SRCS at the FLIS record, which a new cover goes before.
	copy(rec0[offSRCS:offSRCS+4], rec0[offFLIS:offFLIS+4])

	var out bytes.Buffer
	require.NoError(t, Rewrite(bytes.NewReader(src), int64(len(src)), &out, &Metadata{Cover: testImage(t, 40, 60, "image/jpeg")}))
	db := openPDB(t, out.Bytes())
	rec, err := db.record(0, maxHeaderRecord)
	require.NoError(t, err)
	srcs, err := db.record(int(binary.BigEndian.Uint32(rec[offSRCS:])), maxImageRecord)
	require.NoError(t, err)
	assert.Equal(t, "FLIS", string(srcs[:4]))
}

// Some files name their first trailer record as the first image when they
// have no images.
func TestRewrite_FirstImageAtTrailer(t *testing.T) {
	t.Parallel()

	src := testgen.BuildMOBI(t, withoutCover(calibreOptions(testgen.MOBIKindMOBI6)))
	srcDB := openPDB(t, src)
	rec0 := src[srcDB.offsets[0]:]
	copy(rec0[offFirstImage:offFirstImage+4], rec0[offFLIS:offFLIS+4])

	cover := testImage(t, 40, 60, "image/jpeg")
	var out bytes.Buffer
	require.NoError(t, Rewrite(bytes.NewReader(src), int64(len(src)), &out, &Metadata{Cover: cover}))
	assert.Equal(t, cover, parseBytes(t, out.Bytes()).CoverData)
}

// A MOBI6 header's last content record is its last image, so an added cover
// extends it.
func TestRewrite_ExtendsLastContentRecord(t *testing.T) {
	t.Parallel()

	src := testgen.BuildMOBI(t, withoutCover(calibreOptions(testgen.MOBIKindMOBI6)))
	var out bytes.Buffer
	require.NoError(t, Rewrite(bytes.NewReader(src), int64(len(src)), &out, &Metadata{Cover: testImage(t, 40, 60, "image/jpeg")}))
	db := openPDB(t, out.Bytes())
	rec, err := db.record(0, maxHeaderRecord)
	require.NoError(t, err)
	h := headerAt(t, db, 0)
	offset, _ := h.number(exthCoverOffset)
	assert.Equal(t, int(h.firstImage+offset), int(binary.BigEndian.Uint16(rec[offLastContent:])))
}
