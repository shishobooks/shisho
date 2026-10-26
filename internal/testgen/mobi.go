package testgen

import (
	"bytes"
	"math/rand/v2"
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

// MOBI6Content is the text of a generated MOBI6 (KF7) book; see
// mobigen.MOBI6Content.
type MOBI6Content = mobigen.MOBI6Content

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
	// The empty paragraph puts the end of the first text record inside a
	// character, so the record carries the bytes that finish it.
	filler := "<p></p>" + strings.Repeat("<p>Ünïcödé filler text keeps the chapter long.</p>", 120)
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

// KF8SampleFont is the font KF8FontSample embeds: a TrueType header and
// bytes that don't compress, so its compressed data runs past the part of
// a FONT record that is obfuscated.
var KF8SampleFont = func() []byte {
	font := make([]byte, 4096)
	rng := rand.New(rand.NewPCG(5, 6)) //nolint:gosec // test data
	for i := range font {
		font[i] = byte(rng.UintN(256))
	}
	copy(font, "\x00\x01\x00\x00")
	return font
}()

// KF8FontSample is KF8Sample with KF8SampleFont embedded, and its
// stylesheet declaring the font and setting the headings in it.
func KF8FontSample(kind MOBIKind) MOBIOptions {
	opts := KF8Sample(kind)
	opts.KF8.Fonts = [][]byte{KF8SampleFont}
	opts.KF8.Styles[0] = `@font-face { font-family: "Sample"; src: url(kindle:embed:font:0); } h1 { font-family: "Sample"; } ` + opts.KF8.Styles[0]
	return opts
}

// KF7Sample is a MOBI6 (KF7) book marked up the way Calibre and KindleGen
// write one: presentational attributes, font elements, unquoted filepos
// links, recindex images, end tags on void elements, self-closed anchors,
// Mobipocket's own elements, and a guide in the head. It has a cover and
// two chapters split by page breaks, the first long enough to span several
// text records; they show an image and link within and across chapters, one
// link naming the page break before the chapter it goes to. Its table of
// contents points at each chapter and at a place inside the first.
func KF7Sample() MOBIOptions {
	// The empty elements put the end of the first text record inside a
	// character, so the record carries the bytes that finish it.
	filler := "<div></div><span></span>" + strings.Repeat("<p>Ünïcödé filler text keeps the chapter long.</p>", 120)
	return MOBIOptions{
		Kind:     MOBIKindMOBI6,
		Title:    "Old Book",
		Authors:  []string{"Kay Author"},
		Language: "en",
		HasCover: true,
		MOBI6: &MOBI6Content{
			Text: `<html><head><guide><reference type="toc" title="Table of Contents" filepos:toc /></guide></head><body>` +
				`{{ch1}}<p height="1em" width="0pt"><font size="7"><b><font color="#ff0000">Chapter One</font></b></font></p>` +
				`<p height="1em" width="0pt">Read <a filepos:ch2>the next chapter</a> or <a filepos:later>later on</a>. Café &amp; crème&nbsp;&mdash; “quotes”.</p>` +
				`<p width="2em"><i>An indented note with </i><i><b>bold</b></i>.</p>` + filler +
				`{{later}}<p height="1em" width="0pt">Later in chapter one.</p>` +
				`<p><img recindex:0 align="baseline" width="50" height="50"></img></p>` +
				`{{ch2}}<mbp:pagebreak/><p height="1em"><font size="7"><b>Chapter Two</b></font></p>` +
				`<p>Back to <a filepos:ch1>the start</a>. Out to <a href="https://example.com/">the web</a>. On to <a filepos:end>the end</a>.<br></br>` +
				`<a name="old"/>A paragraph left open` +
				`<table border="1" valign="top"><tr valign="top"><td valign="top">A cell</td></tr></table>` +
				`<ul width="0pt"><li value="1" height="0pt">One</li><li value="2">Two</li></ul>` +
				`<blockquote height="1em" width="0pt">Quoted.</blockquote><center>Centered.</center>` +
				`<p>Mobipocket marks <mbp:nu>words</mbp:nu> too.</p><p>{{end}}The end.</p>` +
				`<mbp:pagebreak/>{{toc}}<p align="center"><font size="5"><b>Table of Contents</b></font></p>` +
				`<p width="-14pt"><a filepos:ch1>Chapter One</a></p><p width="-14pt"><a filepos:ch2>Chapter Two</a></p>` +
				`<mbp:pagebreak/><a ></a> <a ></a></body></html>`,
			Images: [][]byte{KF8SamplePlate},
		},
		TOC: []MOBITOCEntry{
			{Title: "Chapter One", Target: "ch1", Children: []MOBITOCEntry{{Title: "Later", Target: "later"}}},
			{Title: "Chapter Two", Target: "ch2"},
		},
	}
}
