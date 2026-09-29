package filegen

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/shishobooks/shisho/pkg/covers"
	"github.com/shishobooks/shisho/pkg/identifiers"
	"github.com/shishobooks/shisho/pkg/models"
)

// EPUBGenerator generates EPUB files with modified metadata.
type EPUBGenerator struct{}

// SupportedType returns the file type this generator handles.
func (g *EPUBGenerator) SupportedType() string {
	return models.FileTypeEPUB
}

// Generate creates a modified EPUB at destPath with updated metadata.
func (g *EPUBGenerator) Generate(ctx context.Context, srcPath, destPath string, book *models.Book, file *models.File) error {
	// Open source EPUB
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return NewGenerationError(models.FileTypeEPUB, err, "failed to open source file")
	}
	defer srcFile.Close()

	srcStat, err := srcFile.Stat()
	if err != nil {
		return NewGenerationError(models.FileTypeEPUB, err, "failed to stat source file")
	}

	srcZip, err := zip.NewReader(srcFile, srcStat.Size())
	if err != nil {
		return NewGenerationError(models.FileTypeEPUB, err, "failed to read source EPUB as zip")
	}

	// Create temporary output file
	tmpPath := destPath + ".tmp"
	destFile, err := os.Create(tmpPath)
	if err != nil {
		return NewGenerationError(models.FileTypeEPUB, err, "failed to create destination file")
	}
	defer func() {
		destFile.Close()
		os.Remove(tmpPath) // Clean up temp file if we don't rename it
	}()

	destZip := zip.NewWriter(destFile)

	// Find the OPF file
	var opfFile *zip.File
	srcNames := make(map[string]bool, len(srcZip.File))
	for _, f := range srcZip.File {
		srcNames[f.Name] = true
		if opfFile == nil && filepath.Ext(f.Name) == ".opf" {
			opfFile = f
		}
	}

	if opfFile == nil {
		return NewGenerationError(models.FileTypeEPUB, nil, "no OPF file found in EPUB")
	}
	opfPath := opfFile.Name

	pkg, err := readOPFPackage(opfFile)
	if err != nil {
		return NewGenerationError(models.FileTypeEPUB, err, "failed to modify OPF metadata")
	}

	// Determine if we need to replace the cover
	var newCoverData []byte
	var newCoverMimeType string
	if file.CoverImageFilename != nil && *file.CoverImageFilename != "" {
		// Resolve via the file's parent dir — book.Filepath may be a synthetic
		// organized-folder path that doesn't exist on disk for root-level files.
		coverPath := covers.FileCoverPath(file)
		newCoverData, err = os.ReadFile(coverPath)
		if err == nil {
			if file.CoverMimeType != nil {
				newCoverMimeType = *file.CoverMimeType
			}
		}
	}

	coverInfo := findCoverImage(pkg, opfPath)
	if coverInfo == nil && len(newCoverData) > 0 {
		coverInfo = addCoverImage(pkg, opfPath, srcNames, filepath.Ext(*file.CoverImageFilename), newCoverMimeType)
	}

	opfContent, err := modifyOPF(pkg, book, file, coverInfo, newCoverMimeType)
	if err != nil {
		return NewGenerationError(models.FileTypeEPUB, err, "failed to modify OPF metadata")
	}

	// Process each file in the source EPUB
	for _, srcZipFile := range srcZip.File {
		select {
		case <-ctx.Done():
			return NewGenerationError(models.FileTypeEPUB, ctx.Err(), "generation cancelled")
		default:
		}

		var destFileContent []byte
		var err error

		if srcZipFile.Name == opfPath {
			destFileContent = opfContent
		} else if coverInfo != nil && srcZipFile.Name == coverInfo.path && len(newCoverData) > 0 {
			// Replace cover image
			destFileContent = newCoverData
		} else {
			// Copy file unchanged
			destFileContent, err = readZipFile(srcZipFile)
			if err != nil {
				return NewGenerationError(models.FileTypeEPUB, err, "failed to read file from source EPUB")
			}
		}

		// Write to destination
		destZipFile, err := destZip.CreateHeader(&zip.FileHeader{
			Name:   srcZipFile.Name,
			Method: srcZipFile.Method,
		})
		if err != nil {
			return NewGenerationError(models.FileTypeEPUB, err, "failed to create file in destination EPUB")
		}

		if _, err := destZipFile.Write(destFileContent); err != nil {
			return NewGenerationError(models.FileTypeEPUB, err, "failed to write file to destination EPUB")
		}
	}

	// A cover Shisho added to a package that had none is a new entry.
	if coverInfo != nil && coverInfo.added {
		w, err := destZip.Create(coverInfo.path)
		if err != nil {
			return NewGenerationError(models.FileTypeEPUB, err, "failed to create cover in destination EPUB")
		}
		if _, err := w.Write(newCoverData); err != nil {
			return NewGenerationError(models.FileTypeEPUB, err, "failed to write cover to destination EPUB")
		}
	}

	// Close the zip writer
	if err := destZip.Close(); err != nil {
		return NewGenerationError(models.FileTypeEPUB, err, "failed to finalize destination EPUB")
	}

	// Close the file before renaming
	if err := destFile.Close(); err != nil {
		return NewGenerationError(models.FileTypeEPUB, err, "failed to close destination file")
	}

	// Atomic rename
	if err := os.Rename(tmpPath, destPath); err != nil {
		return NewGenerationError(models.FileTypeEPUB, err, "failed to finalize destination file")
	}

	return nil
}

// coverImageInfo holds information about the cover image in an EPUB.
type coverImageInfo struct {
	path string // zip entry name
	id   string // manifest item id
	// added is set when the package had no cover and Shisho added the
	// manifest item, so the image is a new zip entry rather than a swap.
	added bool
}

// readOPFPackage reads and parses the OPF file.
func readOPFPackage(opfFile *zip.File) (*opfPackage, error) {
	data, err := readZipFile(opfFile)
	if err != nil {
		return nil, err
	}
	var pkg opfPackage
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}
	return &pkg, nil
}

// isEPUB3 reports whether the package declares version 3 or later. A
// missing or 2.x version is EPUB 2.
func isEPUB3(version string) bool {
	major, _, _ := strings.Cut(strings.TrimSpace(version), ".")
	n, err := strconv.Atoi(major)
	return err == nil && n >= 3
}

// opfEntryPath resolves a manifest href to its zip entry name. Hrefs are
// relative to the OPF file and may be percent-encoded.
func opfEntryPath(opfPath, href string) string {
	if unescaped, err := url.PathUnescape(href); err == nil {
		href = unescaped
	}
	return path.Join(path.Dir(opfPath), href)
}

// findCoverImage finds the cover in the parser's order (pkg/epub ParseOPF):
// the manifest item named by the last <meta name="cover" content="ID"/>,
// then the item whose properties include cover-image (EPUB 3), then an item
// with a conventional cover id. Only image items count, so a meta pointing
// at an XHTML cover page is never overwritten with image bytes.
func findCoverImage(pkg *opfPackage, opfPath string) *coverImageInfo {
	found := func(item opfManifestItem) *coverImageInfo {
		return &coverImageInfo{path: opfEntryPath(opfPath, item.Href), id: item.ID}
	}
	isImage := func(item opfManifestItem) bool {
		return strings.HasPrefix(item.MediaType, "image/")
	}

	var coverID string
	for _, meta := range pkg.Metadata.Meta {
		if meta.Name == "cover" && meta.Content != "" {
			coverID = meta.Content
		}
	}
	if coverID != "" {
		for _, item := range pkg.Manifest.Items {
			if item.ID == coverID && isImage(item) {
				return found(item)
			}
		}
	}

	for _, item := range pkg.Manifest.Items {
		if !isImage(item) {
			continue
		}
		properties, _ := item.Attrs.get("", "properties")
		for _, prop := range strings.Fields(properties) {
			if prop == "cover-image" {
				return found(item)
			}
		}
	}

	for _, item := range pkg.Manifest.Items {
		if !isImage(item) {
			continue
		}
		switch strings.ToLower(item.ID) {
		case "cover-image", "cover", "coverimage":
			return found(item)
		}
	}

	return nil
}

// coverExtensions maps the cover media types Shisho stores to a file
// extension for a cover it adds to the package.
var coverExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// addCoverImage adds a manifest item for Shisho's cover to a package that
// has none, marked the way the package's version expects: properties
// "cover-image" for EPUB 3 and <meta name="cover"> for EPUB 2. An existing
// <meta name="cover"> that pointed at a missing or non-image item is
// repointed. It returns nil when the cover's media type is not a known image
// type.
func addCoverImage(pkg *opfPackage, opfPath string, srcNames map[string]bool, coverExt, mimeType string) *coverImageInfo {
	if mimeType == "" {
		mimeType = mime.TypeByExtension(coverExt)
	}
	mimeType, _, _ = strings.Cut(mimeType, ";")
	ext, ok := coverExtensions[strings.TrimSpace(mimeType)]
	if !ok {
		return nil
	}
	mimeType = strings.TrimSpace(mimeType)

	href := "cover" + ext
	for n := 2; srcNames[opfEntryPath(opfPath, href)]; n++ {
		href = "cover-" + strconv.Itoa(n) + ext
	}
	id := uniqueID(usedIDs(pkg), "cover")

	item := opfManifestItem{ID: id, Href: href, MediaType: mimeType}
	epub3 := isEPUB3(pkg.Version)
	if epub3 {
		item.Attrs = opfAttrs{{Name: xml.Name{Local: "properties"}, Value: "cover-image"}}
	}
	pkg.Manifest.Items = append(pkg.Manifest.Items, item)

	repointed := false
	for i, meta := range pkg.Metadata.Meta {
		if meta.Name == "cover" {
			pkg.Metadata.Meta[i].Content = id
			repointed = true
		}
	}
	if !epub3 && !repointed {
		pkg.Metadata.Meta = append(pkg.Metadata.Meta, opfMeta{Name: "cover", Content: id})
	}

	return &coverImageInfo{path: opfEntryPath(opfPath, href), id: id, added: true}
}

// modifyOPF applies the book and file metadata to the parsed OPF and returns
// the marshaled document.
func modifyOPF(pkg *opfPackage, book *models.Book, file *models.File, coverInfo *coverImageInfo, newCoverMimeType string) ([]byte, error) {
	// Determine title - prefer file.Name over book.Title
	title := book.Title
	if file != nil && file.Name != nil && *file.Name != "" {
		title = *file.Name
	}

	// Update title. A retitled element drops its unmodeled attributes
	// (xml:lang, dir, opf:file-as): they described the old text.
	if len(pkg.Metadata.Titles) > 0 {
		if pkg.Metadata.Titles[0].Text != title {
			pkg.Metadata.Titles[0].Text = title
			pkg.Metadata.Titles[0].Attrs = nil
		}
	} else {
		pkg.Metadata.Titles = []opfTitle{{Text: title}}
	}

	// Update subtitle if present (using a refinement meta tag)
	// First, remove existing subtitle refinements
	var newMetas []opfMeta
	for _, meta := range pkg.Metadata.Meta {
		// Keep meta tags that aren't subtitle refinements
		if meta.Property != "title-type" || meta.Text != "subtitle" {
			newMetas = append(newMetas, meta)
		}
	}
	pkg.Metadata.Meta = newMetas

	// Add subtitle as a second title if book has a subtitle
	if book.Subtitle != nil && *book.Subtitle != "" {
		// Check if we already have multiple titles
		if len(pkg.Metadata.Titles) < 2 {
			pkg.Metadata.Titles = append(pkg.Metadata.Titles, opfTitle{
				Text: *book.Subtitle,
				ID:   "subtitle",
			})
		} else if pkg.Metadata.Titles[1].Text != *book.Subtitle {
			pkg.Metadata.Titles[1].Text = *book.Subtitle
			pkg.Metadata.Titles[1].Attrs = nil
		}
	}

	// Update description if book has one
	if book.Description != nil && *book.Description != "" {
		pkg.Metadata.Description = *book.Description
	}

	// Update publisher from file if available
	if file != nil && file.Publisher != nil {
		pkg.Metadata.Publisher = file.Publisher.Name
	}

	// Update release date from file if available
	if file != nil && file.ReleaseDate != nil {
		pkg.Metadata.Date = file.ReleaseDate.Format("2006-01-02")
	}

	// Update language from file if available
	if file != nil && file.Language != nil && *file.Language != "" {
		pkg.Metadata.Language = *file.Language
		// The package's xml:lang is the default language of its text, so
		// it follows the file's language when the source declared one.
		for i, attr := range pkg.Attrs {
			if attr.Name.Space == xmlNamespace && attr.Name.Local == "lang" {
				pkg.Attrs[i].Value = *file.Language
			}
		}
	}

	// Update authors: keep non-author creators and replace the authors with
	// the book's. A creator's role is its role attribute (EPUB 2) or its
	// refining meta (EPUB 3); one with no role counts as an author. A role
	// refinement counts only in the MARC relator vocabulary (no scheme or
	// scheme="marc:relators"): a code from another list, such as ONIX A01
	// for an author, cannot be read here, so that creator is treated like
	// one with no role instead of kept beside the book's author.
	var newCreators []opfCreator
	var sourceAuthors []opfCreator
	for _, creator := range pkg.Metadata.Creators {
		role := creator.Role
		if role == "" && creator.ID != "" {
			if meta := findRefinement(pkg.Metadata.Meta, creator.ID, "role"); meta != nil {
				if scheme, _ := meta.Attrs.get("", "scheme"); scheme == "" || scheme == "marc:relators" {
					role = strings.TrimSpace(meta.Text)
				}
			}
		}
		if role != "" && role != "aut" {
			newCreators = append(newCreators, creator)
		} else {
			sourceAuthors = append(sourceAuthors, creator)
		}
	}

	// Add book authors sorted by sort order. An author already in the
	// source keeps its element (id, attributes, refinements) so only the
	// values Shisho sets change.
	reused := make([]bool, len(sourceAuthors))
	if len(book.Authors) > 0 {
		authors := make([]*models.Author, len(book.Authors))
		copy(authors, book.Authors)
		sort.Slice(authors, func(i, j int) bool {
			return authors[i].SortOrder < authors[j].SortOrder
		})
		for _, a := range authors {
			if a.Person == nil {
				continue
			}
			creator := opfCreator{Text: a.Person.Name}
			for i, src := range sourceAuthors {
				if !reused[i] && strings.TrimSpace(src.Text) == strings.TrimSpace(a.Person.Name) {
					creator = src
					reused[i] = true
					break
				}
			}
			creator.Role = "aut"
			if a.Person.SortName != "" {
				creator.FileAs = a.Person.SortName
			}
			newCreators = append(newCreators, creator)
		}
	}
	pkg.Metadata.Creators = newCreators

	// Elements Shisho removed take their refinements with them, or the
	// package is left with metas refining ids that no longer exist.
	removedIDs := map[string]bool{}
	for i, src := range sourceAuthors {
		if !reused[i] && src.ID != "" {
			removedIDs[src.ID] = true
		}
	}

	// Update series - using both Calibre meta tags and EPUB3 properties
	// First, remove existing series meta tags (both formats)
	var filteredMetas []opfMeta
	for _, meta := range pkg.Metadata.Meta {
		// Skip Calibre series tags
		if meta.Name == "calibre:series" || meta.Name == "calibre:series_index" {
			continue
		}
		// Skip EPUB3 series tags
		if meta.Property == "belongs-to-collection" || meta.Property == "collection-type" || meta.Property == "group-position" {
			continue
		}
		filteredMetas = append(filteredMetas, meta)
	}

	// Add series info sorted by sort order
	if len(book.BookSeries) > 0 {
		series := make([]*models.BookSeries, len(book.BookSeries))
		copy(series, book.BookSeries)
		sort.Slice(series, func(i, j int) bool {
			return series[i].SortOrder < series[j].SortOrder
		})

		// For primary series (first one), add both Calibre and EPUB3 metadata
		first := series[0]
		if first.Series != nil {
			// Calibre-style (for Calibre compatibility)
			filteredMetas = append(filteredMetas, opfMeta{
				Name:    "calibre:series",
				Content: first.Series.Name,
			})
			if first.SeriesNumber != nil {
				filteredMetas = append(filteredMetas, opfMeta{
					Name:    "calibre:series_index",
					Content: formatFloat(*first.SeriesNumber),
				})
			}

			// EPUB3-style (for Kobo and other modern readers)
			// Uses id and refines attributes to link the metadata together
			filteredMetas = append(filteredMetas, opfMeta{
				Property: "belongs-to-collection",
				ID:       "series-1",
				Text:     first.Series.Name,
			})
			filteredMetas = append(filteredMetas, opfMeta{
				Refines:  "#series-1",
				Property: "collection-type",
				Text:     "series",
			})
			if first.SeriesNumber != nil {
				filteredMetas = append(filteredMetas, opfMeta{
					Refines:  "#series-1",
					Property: "group-position",
					Text:     formatFloat(*first.SeriesNumber),
				})
			}
		}
	}

	// Update genres - replace all dc:subject elements if book has genres
	if len(book.BookGenres) > 0 {
		var newSubjects []string
		for _, bg := range book.BookGenres {
			if bg.Genre != nil {
				newSubjects = append(newSubjects, bg.Genre.Name)
			}
		}
		pkg.Metadata.Subjects = newSubjects
	}
	// If no book genres, preserve existing Subjects (already in pkg.Metadata)

	// Update tags - using Calibre meta tag (comma-separated)
	// Remove existing calibre:tags meta if we're updating tags
	var finalMetas []opfMeta
	var existingCalibreTags string
	for _, meta := range filteredMetas {
		if meta.Name == "calibre:tags" {
			existingCalibreTags = meta.Content
		} else {
			finalMetas = append(finalMetas, meta)
		}
	}

	// Add new calibre:tags if we have tags, or preserve existing
	if len(book.BookTags) > 0 {
		var tagNames []string
		for _, bt := range book.BookTags {
			if bt.Tag != nil {
				tagNames = append(tagNames, bt.Tag.Name)
			}
		}
		if len(tagNames) > 0 {
			finalMetas = append(finalMetas, opfMeta{
				Name:    "calibre:tags",
				Content: strings.Join(tagNames, ", "),
			})
		}
	} else if existingCalibreTags != "" {
		// Preserve existing tags if book has no tags
		finalMetas = append(finalMetas, opfMeta{
			Name:    "calibre:tags",
			Content: existingCalibreTags,
		})
	}

	// Remove and add URL meta tag if file has one
	var filteredForURL []opfMeta
	for _, meta := range finalMetas {
		if meta.Name != "shisho:url" {
			filteredForURL = append(filteredForURL, meta)
		}
	}
	if file != nil && file.URL != nil && *file.URL != "" {
		filteredForURL = append(filteredForURL, opfMeta{
			Name:    "shisho:url",
			Content: *file.URL,
		})
	}
	finalMetas = filteredForURL

	pkg.Metadata.Meta = finalMetas

	// Update identifiers from file
	if file != nil && len(file.Identifiers) > 0 {
		replaced := replaceIdentifiers(pkg.Metadata.Identifiers, pkg.UniqueIdentifier, file.Identifiers)
		kept := map[string]bool{}
		for _, id := range replaced {
			kept[id.ID] = true
		}
		for _, id := range pkg.Metadata.Identifiers {
			if id.ID != "" && !kept[id.ID] {
				removedIDs[id.ID] = true
			}
		}
		pkg.Metadata.Identifiers = replaced
	}

	if len(removedIDs) > 0 {
		var metas []opfMeta
		for _, meta := range pkg.Metadata.Meta {
			if !removedIDs[strings.TrimPrefix(strings.TrimSpace(meta.Refines), "#")] {
				metas = append(metas, meta)
			}
		}
		pkg.Metadata.Meta = metas
	}

	writeRefinements(pkg)

	// Update cover mime type in manifest if we're replacing the cover
	if coverInfo != nil && newCoverMimeType != "" {
		for i, item := range pkg.Manifest.Items {
			if item.ID == coverInfo.id {
				pkg.Manifest.Items[i].MediaType = newCoverMimeType
				break
			}
		}
	}

	// Marshal back to XML
	output, err := xml.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return nil, err
	}

	// Add XML declaration
	result := append([]byte(xml.Header), output...)
	return result, nil
}

// opfNamespace is the OPF namespace. EPUB 2 puts role, file-as, and scheme
// in it (opf:role) on Dublin Core elements.
const opfNamespace = "http://www.idpf.org/2007/opf"

// writeRefinements writes the creator and identifier Role, FileAs, and
// Scheme fields the way the package version expects, then clears them.
// Those fields parse role, file-as, and scheme in any namespace, and they
// must be empty by marshal time: as plain attributes they would print
// without the opf: prefix.
//
// EPUB 2 puts them on the element as opf:role, opf:file-as, and opf:scheme.
// EPUB 3 does not allow them on Dublin Core elements; it states them with
// <meta refines="#id" property="role|file-as|identifier-type">. An existing
// refinement is updated in place, so its own attributes survive; an
// identifier-type refinement is left alone, since the source's value may
// use another vocabulary (such as ONIX codes) for the same type.
func writeRefinements(pkg *opfPackage) {
	type creatorRef struct {
		c      *opfCreator
		idBase string
	}
	creators := make([]creatorRef, 0, len(pkg.Metadata.Creators)+len(pkg.Metadata.Contributors))
	for i := range pkg.Metadata.Creators {
		creators = append(creators, creatorRef{&pkg.Metadata.Creators[i], "creator"})
	}
	for i := range pkg.Metadata.Contributors {
		creators = append(creators, creatorRef{&pkg.Metadata.Contributors[i], "contributor"})
	}

	if !isEPUB3(pkg.Version) {
		for _, ref := range creators {
			c := ref.c
			c.Attrs.setOPF("role", c.Role)
			c.Attrs.setOPF("file-as", c.FileAs)
			c.Role, c.FileAs = "", ""
		}
		for i := range pkg.Metadata.Identifiers {
			id := &pkg.Metadata.Identifiers[i]
			id.Attrs.setOPF("scheme", id.Scheme)
			id.Scheme = ""
		}
		return
	}

	used := usedIDs(pkg)
	for _, ref := range creators {
		c := ref.c
		if (c.Role != "" || c.FileAs != "") && c.ID == "" {
			c.ID = uniqueID(used, ref.idBase)
		}
		if c.Role != "" {
			setRefinement(pkg, c.ID, "role", c.Role, "marc:relators", true)
		}
		if c.FileAs != "" {
			setRefinement(pkg, c.ID, "file-as", c.FileAs, "", true)
		}
		c.Role, c.FileAs = "", ""
	}
	// A title Shisho did not change keeps its attributes, so an EPUB 3
	// source's opf:file-as on dc:title (invalid there) moves into a
	// refinement too.
	for i := range pkg.Metadata.Titles {
		t := &pkg.Metadata.Titles[i]
		fileAs, ok := t.Attrs.get(opfNamespace, "file-as")
		if !ok {
			continue
		}
		t.Attrs.setOPF("file-as", "")
		if fileAs == "" {
			continue
		}
		if t.ID == "" {
			t.ID = uniqueID(used, "title")
		}
		setRefinement(pkg, t.ID, "file-as", fileAs, "", false)
	}
	for i := range pkg.Metadata.Identifiers {
		id := &pkg.Metadata.Identifiers[i]
		if id.Scheme != "" {
			if id.ID == "" {
				id.ID = uniqueID(used, "identifier")
			}
			setRefinement(pkg, id.ID, "identifier-type", id.Scheme, "", false)
		}
		id.Scheme = ""
	}
}

// findRefinement returns the meta refining id with property, or nil.
func findRefinement(metas []opfMeta, id, property string) *opfMeta {
	for i := range metas {
		if strings.TrimSpace(metas[i].Refines) == "#"+id && metas[i].Property == property {
			return &metas[i]
		}
	}
	return nil
}

// setRefinement makes the meta refining id with property hold value. An
// existing meta with that value is kept as is. With overwrite, one holding
// another value takes the new value and drops its attributes (such as a
// scheme or xml:lang), which described the old value; without it, the
// existing meta wins.
func setRefinement(pkg *opfPackage, id, property, value, scheme string, overwrite bool) {
	meta := findRefinement(pkg.Metadata.Meta, id, property)
	if meta != nil {
		if !overwrite || strings.TrimSpace(meta.Text) == value {
			return
		}
		meta.Text = value
		meta.Attrs = nil
	} else {
		pkg.Metadata.Meta = append(pkg.Metadata.Meta, opfMeta{Refines: "#" + id, Property: property, Text: value})
		meta = &pkg.Metadata.Meta[len(pkg.Metadata.Meta)-1]
	}
	if scheme != "" {
		meta.Attrs = append(meta.Attrs, xml.Attr{Name: xml.Name{Local: "scheme"}, Value: scheme})
	}
}

// usedIDs collects the ids in the package so generated ones do not collide.
func usedIDs(pkg *opfPackage) map[string]bool {
	used := map[string]bool{}
	add := func(id string, attrs opfAttrs) {
		if id != "" {
			used[id] = true
		}
		if v, ok := attrs.get("", "id"); ok {
			used[v] = true
		}
	}
	add("", pkg.Attrs)
	for _, t := range pkg.Metadata.Titles {
		add(t.ID, t.Attrs)
	}
	for _, c := range pkg.Metadata.Creators {
		add(c.ID, c.Attrs)
	}
	for _, c := range pkg.Metadata.Contributors {
		add(c.ID, c.Attrs)
	}
	for _, id := range pkg.Metadata.Identifiers {
		add(id.ID, id.Attrs)
	}
	for _, m := range pkg.Metadata.Meta {
		add(m.ID, m.Attrs)
	}
	for _, item := range pkg.Manifest.Items {
		add(item.ID, item.Attrs)
	}
	add("", pkg.Spine.Attrs)
	for _, item := range pkg.Spine.Items {
		add("", item.Attrs)
	}
	return used
}

// uniqueID returns base, or base-2, base-3, ... if it is taken, and marks
// the result used.
func uniqueID(used map[string]bool, base string) string {
	id := base
	for n := 2; used[id]; n++ {
		id = base + "-" + strconv.Itoa(n)
	}
	used[id] = true
	return id
}

// replaceIdentifiers swaps the package's identifiers for the file's, keeping
// the element package@unique-identifier points at. That element is the
// publication's stable identity, and dropping it leaves the package pointing
// at a missing id, which is invalid. Its value follows the file's
// identifiers, though: a file identifier with the same normalized value is
// not written twice (so "urn:isbn:978..." and "978..." count as one), and
// when the file has a different identifier of the same kind, such as a
// corrected ISBN, that value replaces the stale one under the unique id.
func replaceIdentifiers(existing []opfID, uniqueID string, fileIdentifiers []*models.FileIdentifier) []opfID {
	var unique *opfID
	if uniqueID != "" {
		for i := range existing {
			if existing[i].ID == uniqueID {
				kept := existing[i]
				unique = &kept
				break
			}
		}
	}

	newID := func(id *models.FileIdentifier) opfID {
		return opfID{Text: id.Value, Scheme: identifierTypeToScheme(id.Type)}
	}
	if unique == nil {
		result := make([]opfID, 0, len(fileIdentifiers))
		for _, id := range fileIdentifiers {
			result = append(result, newID(id))
		}
		return result
	}

	uniqueType := identifiers.DetectType(unique.Text, unique.Scheme)
	uniqueKey := identifiers.NormalizeValue(string(uniqueType), unique.Text)
	sameValue := func(id *models.FileIdentifier) bool {
		if uniqueType == identifiers.TypeUnknown {
			return strings.TrimSpace(id.Value) == strings.TrimSpace(unique.Text)
		}
		return identifierFamily(id.Type) == identifierFamily(string(uniqueType)) &&
			identifiers.NormalizeValue(id.Type, id.Value) == uniqueKey
	}

	// Which file identifier the unique element absorbs: one with the same
	// value first, otherwise the first of the same kind.
	absorbed := -1
	for i, id := range fileIdentifiers {
		if sameValue(id) {
			absorbed = i
			break
		}
	}
	if absorbed == -1 && uniqueType != identifiers.TypeUnknown {
		for i, id := range fileIdentifiers {
			if identifierFamily(id.Type) == identifierFamily(string(uniqueType)) {
				unique.Text = id.Value
				absorbed = i
				break
			}
		}
	}

	result := []opfID{*unique}
	for i, id := range fileIdentifiers {
		if i != absorbed {
			result = append(result, newID(id))
		}
	}
	return result
}

// identifierFamily groups identifier types that name the same thing, so an
// ISBN-10 unique identifier can take a corrected ISBN-13.
func identifierFamily(idType string) string {
	t := identifiers.Type(idType)
	if t == identifiers.TypeISBN10 || t == identifiers.TypeISBN13 {
		return "isbn"
	}
	return idType
}

// readZipFile reads the contents of a zip file entry.
func readZipFile(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}

// formatFloat formats a float64 for series index.
// Whole numbers display without decimal (e.g., "1"), decimals are preserved (e.g., "1.5").
func formatFloat(f float64) string {
	if f == math.Floor(f) {
		return strconv.Itoa(int(f))
	}
	return fmt.Sprintf("%g", f)
}

// identifierTypeToScheme converts an identifier type to an OPF scheme attribute.
func identifierTypeToScheme(idType string) string {
	switch idType {
	case "isbn_10", "isbn_13":
		return "ISBN"
	case "asin":
		return "ASIN"
	case "uuid":
		return "UUID"
	case "goodreads":
		return "GOODREADS"
	case "google":
		return "GOOGLE"
	default:
		return ""
	}
}

// OPF XML structures for parsing and modifying EPUB metadata.
// These mirror the structure in pkg/epub/opf.go but are simplified for modification.

// Go's encoding/xml resolves prefixes during parsing: elements appear to Go
// under their namespace URI, not their source prefix. For marshaling to
// produce elements that pass a strict namespace check (e.g. foliate-js's
// `el.namespaceURI === NS.DC`), struct tags must declare the namespace URI
// explicitly — e.g. `xml:"http://purl.org/dc/elements/1.1/ title"`.
// Emitting the plain local name `<title>` without a namespace is what broke
// the in-app reader and any other strict EPUB consumer.
type opfPackage struct {
	XMLName          xml.Name    `xml:"http://www.idpf.org/2007/opf package"`
	Version          string      `xml:"version,attr"`
	UniqueIdentifier string      `xml:"unique-identifier,attr,omitempty"`
	Metadata         opfMetadata `xml:"metadata"`
	Manifest         opfManifest `xml:"manifest"`
	Spine            opfSpine    `xml:"spine"`
	Guide            *opfGuide   `xml:"guide,omitempty"`
	Attrs            opfAttrs    `xml:",any,attr"`
}

type opfMetadata struct {
	XMLName      xml.Name     `xml:"metadata"`
	Titles       []opfTitle   `xml:"http://purl.org/dc/elements/1.1/ title"`
	Creators     []opfCreator `xml:"http://purl.org/dc/elements/1.1/ creator"`
	Identifiers  []opfID      `xml:"http://purl.org/dc/elements/1.1/ identifier"`
	Language     string       `xml:"http://purl.org/dc/elements/1.1/ language,omitempty"`
	Publisher    string       `xml:"http://purl.org/dc/elements/1.1/ publisher,omitempty"`
	Date         string       `xml:"http://purl.org/dc/elements/1.1/ date,omitempty"`
	Description  string       `xml:"http://purl.org/dc/elements/1.1/ description,omitempty"`
	Rights       string       `xml:"http://purl.org/dc/elements/1.1/ rights,omitempty"`
	Meta         []opfMeta    `xml:"meta"`
	Subjects     []string     `xml:"http://purl.org/dc/elements/1.1/ subject"`
	Contributors []opfCreator `xml:"http://purl.org/dc/elements/1.1/ contributor"`
}

type opfTitle struct {
	Text  string   `xml:",chardata"`
	ID    string   `xml:"id,attr,omitempty"`
	Attrs opfAttrs `xml:",any,attr"`
}

// opfCreator's Role and FileAs, and opfID's Scheme, parse the attribute in
// any namespace but must be empty at marshal time, or they print without the
// opf: prefix. writeRefinements moves them to the right place and clears
// them; any new marshal path must call it.
type opfCreator struct {
	Text   string   `xml:",chardata"`
	ID     string   `xml:"id,attr,omitempty"`
	Role   string   `xml:"role,attr,omitempty"`
	FileAs string   `xml:"file-as,attr,omitempty"`
	Attrs  opfAttrs `xml:",any,attr"`
}

type opfID struct {
	Text   string   `xml:",chardata"`
	ID     string   `xml:"id,attr,omitempty"`
	Scheme string   `xml:"scheme,attr,omitempty"`
	Attrs  opfAttrs `xml:",any,attr"`
}

type opfMeta struct {
	Text     string   `xml:",chardata"`
	Name     string   `xml:"name,attr,omitempty"`
	Content  string   `xml:"content,attr,omitempty"`
	ID       string   `xml:"id,attr,omitempty"`
	Refines  string   `xml:"refines,attr,omitempty"`
	Property string   `xml:"property,attr,omitempty"`
	Attrs    opfAttrs `xml:",any,attr"`
}

type opfManifest struct {
	XMLName xml.Name          `xml:"manifest"`
	Items   []opfManifestItem `xml:"item"`
}

type opfManifestItem struct {
	ID        string   `xml:"id,attr"`
	Href      string   `xml:"href,attr"`
	MediaType string   `xml:"media-type,attr"`
	Attrs     opfAttrs `xml:",any,attr"`
}

type opfSpine struct {
	XMLName xml.Name       `xml:"spine"`
	Toc     string         `xml:"toc,attr,omitempty"`
	Items   []opfSpineItem `xml:"itemref"`
	Attrs   opfAttrs       `xml:",any,attr"`
}

type opfSpineItem struct {
	IDRef string   `xml:"idref,attr"`
	Attrs opfAttrs `xml:",any,attr"`
}

type opfGuide struct {
	XMLName    xml.Name            `xml:"guide"`
	References []opfGuideReference `xml:"reference"`
}

type opfGuideReference struct {
	Type  string `xml:"type,attr"`
	Href  string `xml:"href,attr"`
	Title string `xml:"title,attr,omitempty"`
}

// xmlNamespace is the namespace encoding/xml gives the xml: prefix.
const xmlNamespace = "http://www.w3.org/XML/1998/namespace"

// opfAttrs carries the attributes an OPF struct does not model by name, so
// the generator round-trips them instead of dropping them. Without it,
// manifest properties="nav" and "cover-image", itemref linear and
// properties, and package prefix/xml:lang were lost on every generated EPUB.
type opfAttrs []xml.Attr

// UnmarshalXMLAttr records every leftover attribute except namespace
// declarations. encoding/xml reports xmlns and xmlns:* as ordinary attributes
// on decode but cannot re-emit them: the marshaler prints them as a duplicate
// xmlns attribute or under an invented "_xmlns" prefix, which is malformed.
// It already declares the namespaces the struct tags need.
func (a *opfAttrs) UnmarshalXMLAttr(attr xml.Attr) error {
	if attr.Name.Space == "xmlns" || (attr.Name.Space == "" && attr.Name.Local == "xmlns") {
		return nil
	}
	*a = append(*a, attr)
	return nil
}

// get returns the value of the attribute with the given namespace and name.
func (a opfAttrs) get(space, local string) (string, bool) {
	for _, attr := range a {
		if attr.Name.Space == space && attr.Name.Local == local {
			return attr.Value, true
		}
	}
	return "", false
}

// setOPF replaces any attribute named local, in any namespace, with
// opf:local set to value, or removes it when value is empty. Clearing the
// other spellings keeps an attribute from printing twice.
func (a *opfAttrs) setOPF(local, value string) {
	kept := (*a)[:0]
	for _, attr := range *a {
		if attr.Name.Local != local {
			kept = append(kept, attr)
		}
	}
	*a = kept
	if value != "" {
		*a = append(*a, xml.Attr{Name: xml.Name{Space: opfNamespace, Local: local}, Value: value})
	}
}
