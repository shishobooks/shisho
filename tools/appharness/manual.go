package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
)

// manualChapters are the PDF fixture's outline, which Shisho keeps as the
// File's chapters, and the 0-based page each starts on. The flows go to
// them by title.
var manualChapters = []struct {
	Title string
	Page  int
}{
	{"Getting Started", 0},
	{"Everyday Use", 2},
	{"Troubleshooting", 4},
}

// manualPages is how many pages the PDF fixture has.
const manualPages = 6

// writeManual writes the PDF the page reader's flows read: numbered pages,
// each saying its chapter in large type, with an outline of
// manualChapters. Hand-written, as the smallest PDF that has text and an
// outline: Helvetica is one of the fonts every PDF reader has.
func writeManual(path, title string) error {
	var b strings.Builder
	var offsets []int
	object := func(body string) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	b.WriteString("%PDF-1.4\n")

	// Objects are numbered in the order written; the page tree, the outline,
	// and the pages refer ahead to numbers worked out here.
	const catalog, pageTree, font, outlines = 1, 2, 3, 4
	firstChapter := outlines + 1
	firstPage := firstChapter + len(manualChapters)
	pageObject := func(page int) int { return firstPage + page*2 }

	object(fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R /Outlines %d 0 R /PageMode /UseOutlines >>", pageTree, outlines))
	kids := make([]string, manualPages)
	for i := range kids {
		kids[i] = fmt.Sprintf("%d 0 R", pageObject(i))
	}
	object(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d /MediaBox [0 0 612 792] >>", strings.Join(kids, " "), manualPages))
	object("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>")
	object(fmt.Sprintf("<< /Type /Outlines /First %d 0 R /Last %d 0 R /Count %d >>",
		firstChapter, firstChapter+len(manualChapters)-1, len(manualChapters)))
	for i, chapter := range manualChapters {
		links := fmt.Sprintf("/Parent %d 0 R", outlines)
		if i > 0 {
			links += fmt.Sprintf(" /Prev %d 0 R", firstChapter+i-1)
		}
		if i < len(manualChapters)-1 {
			links += fmt.Sprintf(" /Next %d 0 R", firstChapter+i+1)
		}
		object(fmt.Sprintf("<< /Title (%s) %s /Dest [%d 0 R /Fit] >>", chapter.Title, links, pageObject(chapter.Page)))
	}
	for page := range manualPages {
		chapter := manualChapters[0].Title
		for _, c := range manualChapters {
			if c.Page <= page {
				chapter = c.Title
			}
		}
		content := fmt.Sprintf("BT /F1 28 Tf 72 700 Td (%s) Tj ET\nBT /F1 160 Tf 220 330 Td (%d) Tj ET\nBT /F1 20 Tf 72 90 Td (%s) Tj ET\n",
			title, page+1, chapter)
		object(fmt.Sprintf("<< /Type /Page /Parent %d 0 R /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>",
			pageTree, font, pageObject(page)+1))
		object(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content))
	}

	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, catalog, xref)
	return errors.WithStack(os.WriteFile(path, []byte(b.String()), 0o600))
}
