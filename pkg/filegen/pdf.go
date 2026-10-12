package filegen

import (
	"context"
	"encoding/xml"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdf/incremental"
)

// noParentPage is the sentinel passed to convertModelChaptersToPDFBookmarks at
// the top level, meaning "no parent constraint, any non-negative page is OK".
const noParentPage = -1

// copyChunk is how much of the original is copied between context checks.
const copyChunk = 1 << 20

// PDFGenerator generates PDF files with modified metadata. The Generated
// File is the Original File byte for byte, followed by an incremental
// update holding the new metadata and bookmarks, so the file is never
// rewritten and a digital signature over the original bytes stays valid.
type PDFGenerator struct{}

// SupportedType returns the file type this generator handles.
func (g *PDFGenerator) SupportedType() string {
	return models.FileTypePDF
}

// Generate writes the Generated File to destPath.
func (g *PDFGenerator) Generate(ctx context.Context, srcPath, destPath string, book *models.Book, file *models.File) error {
	return generateToPath(ctx, g, srcPath, destPath, book, file)
}

// Prepare reads the end of the PDF (its cross-reference data and
// trailer), the catalog, and the page tree when there are chapters, and
// builds the update. An encrypted PDF, or one whose cross-reference data
// cannot be read, fails here, so device routes serve the Original File.
func (g *PDFGenerator) Prepare(ctx context.Context, src Source, book *models.Book, file *models.File) (prepared Prepared, err error) {
	// The reader parses files Shisho did not write. A bug it trips on must
	// still fail preparation cleanly, so device routes serve the Original
	// File instead of failing the download.
	defer func() {
		if r := recover(); r != nil {
			prepared = nil
			err = NewGenerationError(models.FileTypePDF, errors.Errorf("panic: %v", r), "failed to read PDF")
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, NewGenerationError(models.FileTypePDF, err, "context cancelled")
	}
	doc, err := incremental.Open(src, src.Size())
	if errors.Is(err, incremental.ErrEncrypted) {
		return nil, NewGenerationError(models.FileTypePDF, err, "encrypted PDFs cannot be updated")
	}
	if err != nil {
		return nil, NewGenerationError(models.FileTypePDF, err, "failed to read PDF cross-reference data")
	}
	update, err := g.buildUpdate(ctx, doc, book, file, time.Now())
	if err != nil {
		return nil, NewGenerationError(models.FileTypePDF, err, "failed to build PDF metadata update")
	}
	return &pdfPrepared{src: src, update: update}, nil
}

// buildUpdate returns the bytes appended to the original: a new Info
// dictionary, an XMP metadata stream, the chapter bookmarks, and a new
// version of the catalog pointing at them.
func (g *PDFGenerator) buildUpdate(ctx context.Context, doc *incremental.Document, book *models.Book, file *models.File, now time.Time) ([]byte, error) {
	catalogRef, catalog, err := doc.Catalog()
	if err != nil {
		return nil, err
	}
	u := doc.NewUpdate()
	newCatalog := catalog.Clone()
	// /Metadata and /Lang arrived in PDF 1.4.
	if !doc.AtLeastVersion("1.4") {
		newCatalog["Version"] = incremental.Name("1.4")
	}

	// Entries Shisho does not set, such as Creator and CreationDate, are
	// kept from the original Info dictionary.
	info := incremental.Dict{}
	if o, err := doc.Resolve(doc.Trailer()["Info"]); err == nil {
		if d, ok := o.(incremental.Dict); ok {
			info = d.Clone()
		}
	}
	props := g.buildProperties(book, file)
	for k, v := range props {
		info[incremental.Name(k)] = incremental.TextString(v)
	}
	info["ModDate"] = incremental.String(pdfDate(now))
	u.SetTrailer("Info", u.Add(info))

	if lang := props["Language"]; lang != "" {
		newCatalog["Lang"] = incremental.TextString(lang)
	}
	lang := ""
	if l, ok := newCatalog["Lang"].(incremental.String); ok {
		lang = l.Text()
	}
	newCatalog["Metadata"] = u.Add(&incremental.Stream{
		Dict: incremental.Dict{"Type": incremental.Name("Metadata"), "Subtype": incremental.Name("XML")},
		Data: buildXMP(info, authorNames(book), lang, now),
	})

	if outlines, ok := g.buildOutlines(ctx, doc, u, file); ok {
		newCatalog["Outlines"] = outlines
	}

	u.Set(catalogRef, newCatalog)
	return u.Bytes()
}

// buildOutlines adds the chapters as bookmarks, replacing any the original
// has. It returns false, leaving the original's bookmarks alone, when
// there are no chapters.
//
// Bookmarks are best-effort: when the page tree cannot be read, the
// download keeps the rest of its metadata, the same way a cover that
// cannot be extracted never blocks a scan.
func (g *PDFGenerator) buildOutlines(ctx context.Context, doc *incremental.Document, u *incremental.Update, file *models.File) (incremental.Ref, bool) {
	if file == nil || len(file.Chapters) == 0 {
		return incremental.Ref{}, false
	}
	pages, err := doc.Pages()
	if err != nil {
		logger.FromContext(ctx).Warn("failed to read the PDF page tree, continuing without bookmarks", logger.Data{
			"category":      "pdf_bookmark_write",
			"error":         err.Error(),
			"file_id":       file.ID,
			"chapter_count": len(file.Chapters),
		})
		return incremental.Ref{}, false
	}
	bookmarks := convertModelChaptersToPDFBookmarks(file.Chapters, len(pages), noParentPage)
	if len(bookmarks) == 0 {
		return incremental.Ref{}, false
	}
	root := u.Reserve()
	first, last, count := addOutlineItems(u, root, bookmarks, pages)
	u.Set(root, incremental.Dict{
		"Type":  incremental.Name("Outlines"),
		"First": first,
		"Last":  last,
		"Count": incremental.Integer(count),
	})
	return root, true
}

// addOutlineItems adds the outline items for bookmarks under parent, all
// open, and returns the first and last items and how many items are
// visible (the items and, recursively, their open descendants).
func addOutlineItems(u *incremental.Update, parent incremental.Ref, bookmarks []pdfBookmark, pages []incremental.Ref) (first, last incremental.Ref, visible int) {
	refs := make([]incremental.Ref, len(bookmarks))
	for i := range bookmarks {
		refs[i] = u.Reserve()
	}
	for i, bm := range bookmarks {
		item := incremental.Dict{
			"Title":  incremental.TextString(bm.Title),
			"Parent": parent,
			"Dest":   incremental.Array{pages[bm.PageFrom-1], incremental.Name("Fit")},
		}
		if i > 0 {
			item["Prev"] = refs[i-1]
		}
		if i < len(refs)-1 {
			item["Next"] = refs[i+1]
		}
		visible++
		if len(bm.Kids) > 0 {
			kidFirst, kidLast, kidCount := addOutlineItems(u, refs[i], bm.Kids, pages)
			item["First"] = kidFirst
			item["Last"] = kidLast
			item["Count"] = incremental.Integer(kidCount)
			visible += kidCount
		}
		u.Set(refs[i], item)
	}
	return refs[0], refs[len(refs)-1], visible
}

// pdfDate formats t as a PDF date string.
func pdfDate(t time.Time) string {
	return "D:" + t.UTC().Format("20060102150405") + "Z"
}

// authorNames returns the book's author names in order.
func authorNames(book *models.Book) []string {
	authors := make([]*models.Author, len(book.Authors))
	copy(authors, book.Authors)
	sort.SliceStable(authors, func(i, j int) bool {
		return authors[i].SortOrder < authors[j].SortOrder
	})
	var names []string
	for _, a := range authors {
		if a.Person != nil && a.Person.Name != "" {
			names = append(names, a.Person.Name)
		}
	}
	return names
}

// buildXMP returns an XMP metadata packet matching the Info dictionary,
// so readers that prefer XMP over the Info dictionary see the same
// metadata. authors lists the authors one by one; without them, the Info
// dictionary's Author string is the only creator.
func buildXMP(info incremental.Dict, authors []string, lang string, now time.Time) []byte {
	text := func(key incremental.Name) string {
		if s, ok := info[key].(incremental.String); ok {
			return s.Text()
		}
		return ""
	}
	if len(authors) == 0 {
		if a := text("Author"); a != "" {
			authors = []string{a}
		}
	}
	var keywordList []string
	for _, k := range strings.Split(text("Keywords"), ",") {
		if k = strings.TrimSpace(k); k != "" {
			keywordList = append(keywordList, k)
		}
	}

	var b strings.Builder
	esc := func(s string) string {
		var e strings.Builder
		_ = xml.EscapeText(&e, []byte(s))
		return e.String()
	}
	alt := func(tag, value string) {
		if value == "" {
			return
		}
		b.WriteString("   <" + tag + "><rdf:Alt><rdf:li xml:lang=\"x-default\">" + esc(value) + "</rdf:li></rdf:Alt></" + tag + ">\n")
	}
	list := func(tag, kind string, values []string) {
		if len(values) == 0 {
			return
		}
		b.WriteString("   <" + tag + "><rdf:" + kind + ">")
		for _, v := range values {
			b.WriteString("<rdf:li>" + esc(v) + "</rdf:li>")
		}
		b.WriteString("</rdf:" + kind + "></" + tag + ">\n")
	}
	simple := func(tag, value string) {
		if value == "" {
			return
		}
		b.WriteString("   <" + tag + ">" + esc(value) + "</" + tag + ">\n")
	}

	date := now.UTC().Format(time.RFC3339)
	b.WriteString("<?xpacket begin=\"\uFEFF\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>\n")
	b.WriteString("<x:xmpmeta xmlns:x=\"adobe:ns:meta/\">\n")
	b.WriteString(" <rdf:RDF xmlns:rdf=\"http://www.w3.org/1999/02/22-rdf-syntax-ns#\">\n")
	b.WriteString("  <rdf:Description rdf:about=\"\"" +
		" xmlns:dc=\"http://purl.org/dc/elements/1.1/\"" +
		" xmlns:pdf=\"http://ns.adobe.com/pdf/1.3/\"" +
		" xmlns:xmp=\"http://ns.adobe.com/xap/1.0/\">\n")
	b.WriteString("   <dc:format>application/pdf</dc:format>\n")
	alt("dc:title", text("Title"))
	list("dc:creator", "Seq", authors)
	alt("dc:description", text("Subject"))
	list("dc:subject", "Bag", keywordList)
	if lang != "" {
		list("dc:language", "Bag", []string{lang})
	}
	simple("pdf:Keywords", text("Keywords"))
	simple("pdf:Producer", text("Producer"))
	simple("xmp:CreatorTool", text("Creator"))
	simple("xmp:ModifyDate", date)
	simple("xmp:MetadataDate", date)
	b.WriteString("  </rdf:Description>\n")
	b.WriteString(" </rdf:RDF>\n")
	b.WriteString("</x:xmpmeta>\n")
	b.WriteString("<?xpacket end=\"w\"?>")
	return []byte(b.String())
}

// pdfPrepared is a PDF ready to be written: the original, then update.
type pdfPrepared struct {
	src    Source
	update []byte
}

func (p *pdfPrepared) Size() (int64, bool) {
	return p.src.Size() + int64(len(p.update)), true
}

func (p *pdfPrepared) Write(ctx context.Context, w io.Writer) error {
	original := io.NewSectionReader(p.src, 0, p.src.Size())
	for {
		if err := ctx.Err(); err != nil {
			return errors.WithStack(err)
		}
		_, err := io.CopyN(w, original, copyChunk)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return errors.WithStack(err)
		}
	}
	_, err := w.Write(p.update)
	return errors.WithStack(err)
}

// pdfBookmark is a chapter as a PDF bookmark. PageFrom is 1-indexed.
type pdfBookmark struct {
	Title    string
	PageFrom int
	Kids     []pdfBookmark
}

// convertModelChaptersToPDFBookmarks converts database chapter models into
// bookmark entries, preserving nesting.
//
// Chapters are filtered and sorted so the outline reads in page order
// (siblings are monotonically non-decreasing by page, and a child does not
// start before its parent). Specifically:
//   - nil StartPage, negative StartPage, and StartPage outside [0, pageCount)
//     are dropped
//   - children whose StartPage is less than their parent's are dropped
//   - siblings are sorted by StartPage (ties broken by SortOrder for stability)
//
// Rejecting *all* children that start before their parent is simpler than
// checking only the first and avoids surprising subtrees where child[0]
// passes the parent check but child[2] would break page order after
// sorting.
//
// Chapters with invalid StartPage are dropped along with their entire subtree
// — we do not re-parent grandchildren to a dropped parent's parent. In
// practice, PDFs produced by the UI today are flat, so this only matters for
// sidecar- or plugin-supplied nested chapters.
//
// Pages are converted from the 0-indexed storage format to 1-indexed page
// numbers. pageCount is the number of pages in the PDF itself, so every
// bookmark points at a page that exists. parentPage is the parent
// bookmark's 0-indexed StartPage, or noParentPage at the top level.
func convertModelChaptersToPDFBookmarks(chapters []*models.Chapter, pageCount int, parentPage int) []pdfBookmark {
	if len(chapters) == 0 {
		return nil
	}

	// First pass: filter out invalid / out-of-range / child-before-parent entries.
	valid := make([]*models.Chapter, 0, len(chapters))
	for _, ch := range chapters {
		if ch.StartPage == nil {
			continue
		}
		page := *ch.StartPage
		if page < 0 {
			continue
		}
		if page >= pageCount {
			continue
		}
		if parentPage != noParentPage && page < parentPage {
			continue
		}
		valid = append(valid, ch)
	}

	// Sort by StartPage; fall back to SortOrder for ties to keep output stable.
	sort.SliceStable(valid, func(i, j int) bool {
		if *valid[i].StartPage != *valid[j].StartPage {
			return *valid[i].StartPage < *valid[j].StartPage
		}
		return valid[i].SortOrder < valid[j].SortOrder
	})

	result := make([]pdfBookmark, 0, len(valid))
	for _, ch := range valid {
		page := *ch.StartPage
		bm := pdfBookmark{
			Title:    ch.Title,
			PageFrom: page + 1, // 1-indexed
		}
		if len(ch.Children) > 0 {
			bm.Kids = convertModelChaptersToPDFBookmarks(ch.Children, pageCount, page)
		}
		result = append(result, bm)
	}
	return result
}

// buildProperties constructs the info dict map from book and file models.
// Only fields with non-empty values are included; omitted fields keep the
// original's value (Producer and Creator are never set here).
func (g *PDFGenerator) buildProperties(book *models.Book, file *models.File) map[string]string {
	props := make(map[string]string)

	// Title — prefer file.Name over book.Title per the file-level vs
	// book-level convention in pkg/AGENTS.md, matching the M4B generator.
	title := book.Title
	if file != nil && file.Name != nil && *file.Name != "" {
		title = *file.Name
	}
	if title != "" {
		props["Title"] = title
	}

	// Author — join all book authors with ", " sorted by SortOrder.
	if names := authorNames(book); len(names) > 0 {
		props["Author"] = strings.Join(names, ", ")
	}

	// Subject ← book.Description
	if book.Description != nil && *book.Description != "" {
		props["Subject"] = *book.Description
	}

	// Keywords ← tags joined with ", "
	if len(book.BookTags) > 0 {
		var tagNames []string
		for _, bt := range book.BookTags {
			if bt.Tag != nil && bt.Tag.Name != "" {
				tagNames = append(tagNames, bt.Tag.Name)
			}
		}
		if len(tagNames) > 0 {
			props["Keywords"] = strings.Join(tagNames, ", ")
		}
	}

	// Release date is not written: CreationDate is when the file was made,
	// not when the book was published.

	// Language — set in info dict if available.
	if file != nil && file.Language != nil && *file.Language != "" {
		props["Language"] = *file.Language
	}

	return props
}
