package cbz

import (
	"archive/zip"
	"cmp"
	"path/filepath"
	"slices"
	"strings"
)

// PageImages returns the archive's pages in reading order. Every stored page
// number (cover page, chapter start pages) is a 0-indexed position in this
// list, and every consumer that turns a page number into an archive entry (the
// reader's page cache, scan cover extraction, KePub conversion, the ComicInfo
// FrontCover index) must use it, or they disagree on which image is page N.
//
// Pages are the image entries in natural order: digit runs compare as numbers,
// so "page2" comes before "page10", and zero-padded names keep their byte
// order. macOS metadata is not a page: dot-files such as the "._page1.jpg"
// resource forks, and anything under "__MACOSX/".
//
// Changing which entries are pages, or their order, changes which image a
// stored page number names: bump cbzpages.CBZPageKey with it, so cached pages
// in the old order are not served.
func PageImages(zipReader *zip.Reader) []*zip.File {
	var pages []*zip.File
	for _, f := range zipReader.File {
		if IsPageImage(f.Name) {
			pages = append(pages, f)
		}
	}
	slices.SortStableFunc(pages, func(a, b *zip.File) int {
		return naturalCompare(a.Name, b.Name)
	})
	return pages
}

// IsPageImage reports whether an archive entry name is a page image, using
// the same rules as PageImages.
func IsPageImage(name string) bool {
	if strings.HasPrefix(filepath.Base(name), ".") {
		return false
	}
	if slices.Contains(strings.Split(name, "/"), "__MACOSX") {
		return false
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return true
	}
	return false
}

// naturalCompare orders strings naturally by alternating non-digit and digit
// runs: digit runs compare numerically (so "page2" < "page10"), non-digit runs
// compare byte-wise. This correctly orders filenames with multiple numbers,
// e.g. "Foo 365 - c001 - p000.jpg" < "Foo 365 - c001 - p001.jpg". Names that
// differ only in leading zeros ("p01" and "p1") fall back to byte order, so
// the order never depends on archive position.
func naturalCompare(a, b string) int {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		aDigit := a[i] >= '0' && a[i] <= '9'
		bDigit := b[j] >= '0' && b[j] <= '9'

		if aDigit && bDigit {
			aStart := i
			for i < len(a) && a[i] >= '0' && a[i] <= '9' {
				i++
			}
			bStart := j
			for j < len(b) && b[j] >= '0' && b[j] <= '9' {
				j++
			}
			aNum := strings.TrimLeft(a[aStart:i], "0")
			bNum := strings.TrimLeft(b[bStart:j], "0")
			if len(aNum) != len(bNum) {
				return cmp.Compare(len(aNum), len(bNum))
			}
			if c := strings.Compare(aNum, bNum); c != 0 {
				return c
			}
			continue
		}

		if a[i] != b[j] {
			return cmp.Compare(a[i], b[j])
		}
		i++
		j++
	}
	if c := cmp.Compare(len(a)-i, len(b)-j); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}
