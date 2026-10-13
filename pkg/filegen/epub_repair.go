package filegen

import (
	"encoding/xml"
	"html"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
)

// EPUB 3 requires every package to have a dcterms:modified date and a
// navigation document. Sources that lack them, often EPUB 2 books relabeled
// as 3.0, get them in the Generated EPUB so strict readers and validators
// accept it. EPUB 2 packages need neither and get neither.

// addModifiedDate adds a dcterms:modified date to an EPUB 3 package that
// has none: when the book's or the file's metadata last changed, so the same
// metadata always gives the same file, or now when neither is known.
func addModifiedDate(pkg *opfPackage, book *models.Book, file *models.File) {
	if !isEPUB3(pkg.Version) {
		return
	}
	for _, meta := range pkg.Metadata.Meta {
		if meta.Property == "dcterms:modified" && meta.Refines == "" {
			return
		}
	}
	modified := book.UpdatedAt
	if file.UpdatedAt.After(modified) {
		modified = file.UpdatedAt
	}
	if modified.IsZero() {
		modified = time.Now()
	}
	pkg.Metadata.Meta = append(pkg.Metadata.Meta, opfMeta{
		Property: "dcterms:modified",
		Text:     modified.UTC().Format("2006-01-02T15:04:05Z"),
	})
}

// addNavDocument adds a navigation document to the manifest of an EPUB 3
// package that has none, and returns its zip entry name and content, or ""
// when the package needs none or has no document to link to. The nav lists
// the file's chapters, or failing that links the first spine document under
// title.
//
// Stored chapter hrefs are relative to the document the scan read them
// from, which for a package without a nav is the NCX, so the nav goes in the
// NCX's directory and uses them as written.
func addNavDocument(pkg *opfPackage, opfPath, navPath string, srcNames map[string]bool, title string, chapters []*models.Chapter) (string, []byte) {
	if !isEPUB3(pkg.Version) || navPath != "" {
		return "", nil
	}
	dir := path.Dir(opfPath)
	for _, item := range pkg.Manifest.Items {
		if pkg.Spine.Toc != "" && item.ID == pkg.Spine.Toc {
			dir = path.Dir(opfEntryPath(opfPath, item.Href))
			break
		}
	}

	var list strings.Builder
	writeNavList(&list, chapterTree(chapters), title)
	if list.Len() == 0 {
		first := firstSpineDocument(pkg, opfPath)
		if first == "" {
			return "", nil
		}
		list.WriteString(`<ol><li><a href="` + html.EscapeString(entryHref(dir, first)) + `">` + html.EscapeString(title) + "</a></li></ol>")
	}

	name := path.Join(dir, "nav.xhtml")
	for n := 2; srcNames[name]; n++ {
		name = path.Join(dir, "nav-"+strconv.Itoa(n)+".xhtml")
	}
	pkg.Manifest.Items = append(pkg.Manifest.Items, opfManifestItem{
		ID:        uniqueID(usedIDs(pkg), "nav"),
		Href:      entryHref(path.Dir(opfPath), name),
		MediaType: "application/xhtml+xml",
		Attrs:     opfAttrs{{Name: xml.Name{Local: "properties"}, Value: "nav"}},
	})

	var doc strings.Builder
	doc.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head>
<title>`)
	doc.WriteString(html.EscapeString(title))
	doc.WriteString(`</title>
</head>
<body>
<nav epub:type="toc" id="toc">
<h1>`)
	doc.WriteString(html.EscapeString(title))
	doc.WriteString("</h1>\n")
	doc.WriteString(list.String())
	doc.WriteString("\n</nav>\n</body>\n</html>\n")
	return name, []byte(doc.String())
}

// writeNavList writes chapters as a nav ol, leaving out a chapter with no
// href and nothing under it, which a nav cannot show. A chapter with no
// title is labeled with untitled, since a nav entry must have text. Nothing
// is written when no chapter is left.
func writeNavList(b *strings.Builder, chapters []editedChapter, untitled string) {
	var items strings.Builder
	for _, ch := range chapters {
		var children strings.Builder
		writeNavList(&children, ch.children, untitled)
		if ch.href == "" && children.Len() == 0 {
			continue
		}
		label := strings.TrimSpace(ch.title)
		if label == "" {
			label = untitled
		}
		items.WriteString("<li>")
		if ch.href != "" {
			items.WriteString(`<a href="` + html.EscapeString(ch.href) + `">` + html.EscapeString(label) + "</a>")
		} else {
			items.WriteString("<span>" + html.EscapeString(label) + "</span>")
		}
		items.WriteString(children.String())
		items.WriteString("</li>")
	}
	if items.Len() > 0 {
		b.WriteString("<ol>" + items.String() + "</ol>")
	}
}

// firstSpineDocument returns the zip entry name of the first document in the
// spine, or "" when there is none.
func firstSpineDocument(pkg *opfPackage, opfPath string) string {
	for _, ref := range pkg.Spine.Items {
		for _, item := range pkg.Manifest.Items {
			if item.ID == ref.IDRef {
				return opfEntryPath(opfPath, item.Href)
			}
		}
	}
	return ""
}

// entryHref returns the href that names the zip entry target from a
// document in dir, percent-encoded.
func entryHref(dir, target string) string {
	to := strings.Split(target, "/")
	for i, part := range to {
		to[i] = url.PathEscape(part)
	}
	if dir == "." {
		return strings.Join(to, "/")
	}
	from := strings.Split(dir, "/")
	for i, part := range from {
		from[i] = url.PathEscape(part)
	}
	common := 0
	for common < len(from) && common < len(to)-1 && from[common] == to[common] {
		common++
	}
	parts := make([]string, 0, len(from)-common+len(to)-common)
	for range from[common:] {
		parts = append(parts, "..")
	}
	return strings.Join(append(parts, to[common:]...), "/")
}
