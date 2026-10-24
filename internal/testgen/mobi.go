package testgen

import (
	"bytes"
	"strings"
	"testing"

	"github.com/shishobooks/shisho/internal/mobigen"
)

// GenerateMOBI writes a MOBI, AZW3, or combo file shaped like Calibre's
// output and returns its path.
func GenerateMOBI(t *testing.T, dir, filename string, opts MOBIOptions) string {
	t.Helper()
	return WriteFile(t, dir, filename, BuildMOBI(t, opts))
}

// BuildMOBI returns the bytes of a MOBI, AZW3, or combo file; see
// mobigen.Build.
func BuildMOBI(t *testing.T, opts MOBIOptions) []byte {
	t.Helper()
	b, err := mobigen.Build(opts)
	if err != nil {
		t.Fatalf("failed to build MOBI: %v", err)
	}
	return b
}

// MOBIKF8Content is the text of a generated KF8 book; see
// mobigen.KF8Content.
type MOBIKF8Content = mobigen.KF8Content

// MOBIKF8Part is one document of a generated KF8 book.
type MOBIKF8Part = mobigen.KF8Part

// KF8SamplePlate is the image KF8Sample's first chapter shows.
var KF8SamplePlate = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0x42}, 300)...)

// KF8Sample is a KF8 book (an AZW3, or the KF8 half of a combo file) with a
// cover and two chapters: the first is long enough to span several text
// records and is stored as two fragments, and they show an image, use a
// stylesheet that embeds the image too, and link within and across
// chapters, once to an element with no id. The text quotes a kindle: URI,
// which is not a reference. Its table of contents points at each chapter and at a place
// inside the first.
func KF8Sample(kind MOBIKind) MOBIOptions {
	filler := strings.Repeat("<p>Ünïcödé filler text keeps the chapter long.</p>", 120)
	return MOBIOptions{
		Kind:     kind,
		Title:    "Kindle Book",
		Authors:  []string{"Kay Author"},
		Language: "en",
		HasCover: true,
		KF8: &MOBIKF8Content{
			Parts: []MOBIKF8Part{
				{Fragments: []string{
					`<h1 id="ch1" aid="1">Chapter One</h1><p aid="2">Read <a href="kindle:pos:id:ch2">the next chapter</a> or <a href="kindle:pos:id:later">later on</a>.</p>` + filler,
					`<p id="later" aid="3">Later in chapter one.</p><img src="kindle:embed:img:0" alt="plate"/>`,
				}},
				{Fragments: []string{
					`<h1 id="ch2" aid="4">Chapter Two</h1><p>Back to <a href="kindle:pos:id:ch1">the start</a>. Out to <a href="https://example.com/">the web</a>. On to <a href="kindle:pos:id:end">the end</a>.</p>` +
						`<p>A Kindle names images like kindle:embed:0001 inside.</p><p data-anchor="end" aid="5">The end.</p>`,
				}},
			},
			Styles: []string{`h1 { color: red; } body { background: url(kindle:embed:img:0); }`},
			Images: [][]byte{KF8SamplePlate},
		},
		TOC: []MOBITOCEntry{
			{Title: "Chapter One", Target: "ch1", Children: []MOBITOCEntry{{Title: "Later", Target: "later"}}},
			{Title: "Chapter Two", Target: "ch2"},
		},
	}
}
