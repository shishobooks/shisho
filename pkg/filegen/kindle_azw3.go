package filegen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/shishobooks/shisho/pkg/epub"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/identifiers"
	"github.com/shishobooks/shisho/pkg/kepub"
	"github.com/shishobooks/shisho/pkg/mobi"
	"github.com/shishobooks/shisho/pkg/models"
)

// epubToAZW3 is the Conversion step from EPUB to AZW3. It runs the EPUB
// generator's preparation first, so the book's metadata, cover, and chapter
// edits are applied as for any EPUB, and builds a KF8 book from the result:
// its spine documents, stylesheets, images, fonts, table of contents, and
// guide. An AZW3 lists every record's offset before the records, so it is
// built in full before it is sent.
type epubToAZW3 struct{}

func (epubToAZW3) builtInFull() {}

func (epubToAZW3) prepare(ctx context.Context, in stepInput) (Prepared, error) {
	entries := in.entries
	if entries == nil {
		var err error
		if entries, err = plannedEntries(ctx, in, models.FileTypeAZW3); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, NewGenerationError(models.FileTypeAZW3, err, "generation cancelled")
	}
	book, err := kf8Book(entries)
	if err != nil {
		return nil, err
	}
	book.Identifier = kindleFileIdentifier(in.file)
	return &preparedAZW3{book: book}, nil
}

// kindleFileIdentifier returns the identifier written as the ASIN of a
// file delivered as AZW3: a UUID derived from the File, the same on every
// regeneration and different for each File of a book. Reading positions on
// a Kindle follow the filename instead (kindleFilename in pkg/ereader).
func kindleFileIdentifier(file *models.File) string {
	d := sha256.Sum256([]byte(fmt.Sprintf("shisho:file:%d", file.ID)))
	d[6] = d[6]&0x0F | 0x50 // a name-based UUID
	d[8] = d[8]&0x3F | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", d[0:4], d[4:6], d[6:8], d[8:10], d[10:16])
}

// preparedAZW3 is an AZW3 ready to be built.
type preparedAZW3 struct {
	book *mobi.KF8Book
}

func (p *preparedAZW3) build(ctx context.Context, f *os.File) error {
	if err := mobi.WriteKF8(ctx, f, p.book); err != nil {
		return NewGenerationError(models.FileTypeAZW3, err, "failed to convert EPUB: "+err.Error())
	}
	return nil
}

// Size reports no size: the file is measured only once it is built.
func (p *preparedAZW3) Size() (int64, bool) {
	return 0, false
}

// Write builds the file in the system's temporary directory and copies it
// to w. Downloads build it through builtConversion instead, into the
// download cache.
func (p *preparedAZW3) Write(ctx context.Context, w io.Writer) error {
	f, err := fileutils.CreateTemp("", "shisho-azw3-*", 0o600)
	if err != nil {
		return NewGenerationError(models.FileTypeAZW3, err, "failed to create a file to build in")
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := p.build(ctx, f); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return NewGenerationError(models.FileTypeAZW3, err, "failed to write destination AZW3")
	}
	if _, err := io.Copy(w, f); err != nil {
		return NewGenerationError(models.FileTypeAZW3, err, "failed to write destination AZW3")
	}
	return nil
}

// kf8Book reads an EPUB's entries into a book for mobi.WriteKF8. The
// documents, stylesheets, and fonts are read now; the images only as they
// are written.
func kf8Book(entries []kepub.Entry) (*mobi.KF8Book, error) {
	byName := make(map[string]kepub.Entry, len(entries))
	var opfName string
	for _, e := range entries {
		byName[e.Name] = e
		// The EPUB generator takes the first OPF it finds, so this does too.
		if opfName == "" && filepath.Ext(e.Name) == ".opf" {
			opfName = e.Name
		}
	}
	if opfName == "" {
		return nil, NewGenerationError(models.FileTypeAZW3, nil, "no OPF file found in EPUB")
	}
	read := func(name string) ([]byte, error) {
		data, err := byName[name].Data()
		if err != nil {
			return nil, NewGenerationError(models.FileTypeAZW3, err, "failed to read "+name+" from EPUB")
		}
		return data, nil
	}
	opfData, err := read(opfName)
	if err != nil {
		return nil, err
	}
	parsed, err := epub.ParseOPF(opfName, io.NopCloser(bytes.NewReader(opfData)))
	if err != nil {
		return nil, NewGenerationError(models.FileTypeAZW3, err, "failed to read the EPUB's OPF")
	}
	opf, pkg := parsed.OPF, parsed.Package

	book := &mobi.KF8Book{Metadata: kf8Metadata(opf)}
	// entryName returns the entry an href in the OPF names.
	entryName := func(href string) string {
		if unescaped, err := url.PathUnescape(href); err == nil {
			href = unescaped
		}
		return path.Join(path.Dir(opfName), href)
	}
	var obfuscated map[string]string
	if e, ok := byName[encryptionPath]; ok {
		data, err := e.Data()
		if err != nil {
			return nil, NewGenerationError(models.FileTypeAZW3, err, "failed to read "+encryptionPath+" from EPUB")
		}
		obfuscated = obfuscatedFonts(data)
	}
	// Preparation keeps each font keyed on the identifiers the OPF holds.
	keys := epubPackageIdentifiers(pkg)
	items := map[string]string{}
	for _, item := range pkg.Manifest.Item {
		name := entryName(item.Href)
		items[item.ID] = name
		if _, ok := byName[name]; !ok {
			continue
		}
		switch {
		case item.MediaType == "text/css":
			data, err := read(name)
			if err != nil {
				return nil, err
			}
			book.Styles = append(book.Styles, mobi.Document{Name: name, MediaType: item.MediaType, Content: data})
		case strings.HasPrefix(item.MediaType, "image/"):
			e := byName[name]
			book.Images = append(book.Images, mobi.KF8Image{Name: name, MediaType: item.MediaType, Load: e.Data})
		case isFontItem(item.MediaType, name):
			data, err := read(name)
			if err != nil {
				return nil, err
			}
			if algorithm, ok := obfuscated[name]; ok {
				if plain, _, ok := deobfuscateFont(data, algorithm, keys); ok {
					data = plain
				}
			}
			book.Fonts = append(book.Fonts, mobi.Document{Name: name, MediaType: item.MediaType, Content: data})
		}
	}
	for _, ref := range pkg.Spine.Itemref {
		name, ok := items[ref.Idref]
		if _, exists := byName[name]; !ok || !exists {
			continue
		}
		data, err := read(name)
		if err != nil {
			return nil, err
		}
		book.Documents = append(book.Documents, mobi.Document{Name: name, MediaType: "application/xhtml+xml", Content: data})
	}
	if len(book.Documents) == 0 {
		return nil, NewGenerationError(models.FileTypeAZW3, fmt.Errorf("%w: %w", ErrUnsupportedContent, mobi.ErrNoDocuments),
			"this book can't be converted to AZW3: its EPUB has no documents")
	}
	for _, m := range pkg.Metadata.Meta {
		if m.Property == "rendition:layout" && strings.TrimSpace(m.Text) == "pre-paginated" {
			book.FixedLayout = true
			book.OriginalResolution = pageResolution(book.Documents)
		}
	}
	if opf.CoverFilepath != "" {
		// The parser joins the OPF's href as written, still escaped.
		cover := opf.CoverFilepath
		if unescaped, err := url.PathUnescape(cover); err == nil {
			cover = unescaped
		}
		cover = path.Clean(cover)
		if slices.ContainsFunc(book.Images, func(img mobi.KF8Image) bool { return img.Name == cover }) {
			book.Cover = cover
		}
	}

	// The table of contents is the nav document's when it has entries, and
	// the NCX's otherwise, as the scan reads it.
	var navName, ncxName string
	for _, item := range pkg.Manifest.Item {
		if navName == "" && slices.Contains(strings.Fields(item.Properties), "nav") {
			navName = entryName(item.Href)
		}
		if item.ID == pkg.Spine.Toc && pkg.Spine.Toc != "" {
			ncxName = entryName(item.Href)
		}
	}
	var navData []byte
	for _, toc := range []struct {
		name    string
		entries func(*xmlNode, string) []*tocEntry
	}{{navName, navEntries}, {ncxName, ncxEntries}} {
		if _, ok := byName[toc.name]; !ok {
			continue
		}
		data, err := read(toc.name)
		if err != nil {
			return nil, err
		}
		if toc.name == navName {
			navData = data
		}
		if doc := readTOCDoc(toc.name, data, toc.entries); doc.hasChapters() {
			book.TOC = kf8NavPoints(doc.entries)
			break
		}
	}

	// The guide is the OPF's, or, in an EPUB 3 without one, the nav
	// document's landmarks.
	for _, ref := range pkg.Guide.Reference {
		book.Guide = append(book.Guide, mobi.GuideRef{Type: ref.Type, Title: ref.Title, Href: resolveHref(opfName, ref.Href)})
	}
	if len(book.Guide) == 0 {
		// The nav document, when the book has one, was read for the table
		// of contents.
		book.Guide = landmarks(navName, navData)
	}
	return book, nil
}

// landmarkTypes maps an EPUB 3 landmark's type to the guide type a Kindle
// reads.
var landmarkTypes = map[string]string{
	"bodymatter": "text", "toc": "toc", "cover": "cover", "titlepage": "title-page",
	"copyright-page": "copyright-page", "dedication": "dedication", "preface": "preface",
	"foreword": "foreword", "acknowledgments": "acknowledgements", "bibliography": "bibliography",
	"glossary": "glossary", "index": "index", "loi": "loi", "lot": "lot", "colophon": "colophon",
	"epigraph": "epigraph",
}

// landmarks returns the guide references a nav document's landmarks nav
// gives, leaving out types a guide has none of.
func landmarks(navName string, data []byte) []mobi.GuideRef {
	if data == nil {
		return nil
	}
	root, err := parseXMLNodes(data)
	if err != nil || root.name != "html" {
		return nil
	}
	ol := navList(root, "landmarks")
	if ol == nil {
		return nil
	}
	var refs []mobi.GuideRef
	for _, li := range ol.children {
		a := li.child("a")
		if li.name != "li" || a == nil {
			continue
		}
		typ, ok := landmarkTypes[strings.TrimSpace(a.attr("type"))]
		if !ok || a.attr("href") == "" {
			continue
		}
		refs = append(refs, mobi.GuideRef{Type: typ, Title: strings.TrimSpace(a.text.String()), Href: resolveHref(navName, a.attr("href"))})
	}
	return refs
}

// fontExtensions are the extensions of font files, for a manifest that
// gives a font a media type other than a font one.
var fontExtensions = map[string]bool{".ttf": true, ".otf": true, ".woff": true, ".woff2": true}

// isFontItem reports whether a manifest item is a font. EPUBs in the wild
// give fonts many media types, so the item's extension counts too; the
// writer leaves out anything that turns out to be no font.
func isFontItem(mediaType, name string) bool {
	return strings.Contains(mediaType, "font") ||
		mediaType == "application/vnd.ms-opentype" || fontExtensions[strings.ToLower(path.Ext(name))]
}

// kf8NavPoints turns table of contents entries into the writer's, leaving
// out entries the scan skips, those with no title.
func kf8NavPoints(entries []*tocEntry) []*mobi.NavPoint {
	var out []*mobi.NavPoint
	for _, e := range entries {
		if e.title == "" {
			continue
		}
		out = append(out, &mobi.NavPoint{Title: e.title, Href: e.resolved, Children: kf8NavPoints(e.children)})
	}
	return out
}

// kf8Metadata returns what the EXTH block holds, from the OPF the EPUB
// generator wrote, which has Shisho's metadata in it.
func kf8Metadata(opf *epub.OPF) mobi.Metadata {
	meta := mobi.Metadata{
		Title:       opf.Title,
		Description: opf.Description,
		Publisher:   opf.Publisher,
		ReleaseDate: opf.ReleaseDate,
		Genres:      opf.Genres,
	}
	for _, a := range opf.Authors {
		meta.Authors = append(meta.Authors, a.Name)
	}
	if opf.Language != nil {
		meta.Language = *opf.Language
	}
	ids := &mobi.Identifiers{}
	for _, id := range opf.Identifiers {
		if t := identifiers.Type(id.Type); t == identifiers.TypeISBN10 || t == identifiers.TypeISBN13 {
			ids.ISBNs = append(ids.ISBNs, id.Value)
		}
	}
	meta.Identifiers = ids
	return meta
}

var (
	metaTag      = regexp.MustCompile(`(?i)<meta\s[^>]*>`)
	viewportName = regexp.MustCompile(`(?i)\sname\s*=\s*["']viewport["']`)
	metaContent  = regexp.MustCompile(`(?i)\scontent\s*=\s*["']([^"']*)["']`)
	viewportSide = regexp.MustCompile(`(width|height)\s*=\s*(\d+)`)
)

// pageResolution returns the size a fixed-layout book's pages were made
// for, as "1600x1200": the largest width and height any document's viewport
// gives, since pages can differ, such as a landscape spread among portrait
// pages. It is "" when no viewport gives both.
func pageResolution(docs []mobi.Document) string {
	var width, height int
	for _, d := range docs {
		for _, tag := range metaTag.FindAll(d.Content, -1) {
			content := metaContent.FindSubmatch(tag)
			if !viewportName.Match(tag) || content == nil {
				continue
			}
			for _, side := range viewportSide.FindAllSubmatch(content[1], -1) {
				n, err := strconv.Atoi(string(side[2]))
				switch {
				case err != nil:
				case string(side[1]) == "width":
					width = max(width, n)
				default:
					height = max(height, n)
				}
			}
		}
	}
	if width == 0 || height == 0 {
		return ""
	}
	return fmt.Sprintf("%dx%d", width, height)
}
