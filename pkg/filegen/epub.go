package filegen

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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

	// Find the OPF file and cover image info
	var opfPath string
	var coverInfo *coverImageInfo
	for _, f := range srcZip.File {
		if filepath.Ext(f.Name) == ".opf" {
			opfPath = f.Name
			// Parse OPF to find cover image path
			coverInfo, err = findCoverImageInOPF(f)
			if err != nil {
				// Not fatal - we just won't replace the cover
				coverInfo = nil
			}
			break
		}
	}

	if opfPath == "" {
		return NewGenerationError(models.FileTypeEPUB, nil, "no OPF file found in EPUB")
	}

	// Determine if we need to replace the cover
	var newCoverData []byte
	var newCoverMimeType string
	if file.CoverImageFilename != nil && *file.CoverImageFilename != "" {
		// Resolve the full cover path from the book's directory
		// CoverImageFilename is just a filename, we need to find the cover directory
		coverPath := resolveCoverPath(book, file)
		if coverPath != "" {
			newCoverData, err = os.ReadFile(coverPath)
			if err == nil {
				if file.CoverMimeType != nil {
					newCoverMimeType = *file.CoverMimeType
				}
			}
		}
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
			// Modify the OPF file
			destFileContent, err = modifyOPF(srcZipFile, book, file, coverInfo, newCoverMimeType)
			if err != nil {
				return NewGenerationError(models.FileTypeEPUB, err, "failed to modify OPF metadata")
			}
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
	path     string
	mimeType string
	id       string
}

// findCoverImageInOPF finds the cover image path from an OPF file.
func findCoverImageInOPF(opfFile *zip.File) (*coverImageInfo, error) {
	r, err := opfFile.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var pkg opfPackage
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	// Find cover ID from meta tags
	var coverID string
	for _, meta := range pkg.Metadata.Meta {
		if meta.Name == "cover" {
			coverID = meta.Content
			break
		}
	}

	if coverID == "" {
		return nil, nil
	}

	// Find the manifest item with that ID
	basePath := filepath.Dir(opfFile.Name)
	if basePath == "." {
		basePath = ""
	} else {
		basePath += "/"
	}

	for _, item := range pkg.Manifest.Items {
		if item.ID == coverID {
			return &coverImageInfo{
				path:     basePath + item.Href,
				mimeType: item.MediaType,
				id:       coverID,
			}, nil
		}
	}

	return nil, nil
}

// modifyOPF modifies the OPF file with new metadata.
func modifyOPF(opfFile *zip.File, book *models.Book, file *models.File, coverInfo *coverImageInfo, newCoverMimeType string) ([]byte, error) {
	r, err := opfFile.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// Parse the OPF
	var pkg opfPackage
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	// Determine title - prefer file.Name over book.Title
	title := book.Title
	if file != nil && file.Name != nil && *file.Name != "" {
		title = *file.Name
	}

	// Update title
	if len(pkg.Metadata.Titles) > 0 {
		pkg.Metadata.Titles[0].Text = title
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
		} else {
			pkg.Metadata.Titles[1].Text = *book.Subtitle
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

	// Update authors - replace all creators with role="aut"
	var newCreators []opfCreator
	// First, keep non-author creators
	for _, creator := range pkg.Metadata.Creators {
		if creator.Role != "" && creator.Role != "aut" {
			newCreators = append(newCreators, creator)
		}
	}

	// Add book authors sorted by sort order
	if len(book.Authors) > 0 {
		authors := make([]*models.Author, len(book.Authors))
		copy(authors, book.Authors)
		sort.Slice(authors, func(i, j int) bool {
			return authors[i].SortOrder < authors[j].SortOrder
		})
		for _, a := range authors {
			if a.Person != nil {
				newCreators = append(newCreators, opfCreator{
					Text:   a.Person.Name,
					Role:   "aut",
					FileAs: a.Person.SortName,
				})
			}
		}
	}
	pkg.Metadata.Creators = newCreators

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

	// Remove and add imprint meta tag if file has one
	var filteredForImprint []opfMeta
	for _, meta := range finalMetas {
		if meta.Name != "shisho:imprint" {
			filteredForImprint = append(filteredForImprint, meta)
		}
	}
	if file != nil && file.Imprint != nil {
		filteredForImprint = append(filteredForImprint, opfMeta{
			Name:    "shisho:imprint",
			Content: file.Imprint.Name,
		})
	}
	finalMetas = filteredForImprint

	pkg.Metadata.Meta = finalMetas

	// Update identifiers from file
	if file != nil && len(file.Identifiers) > 0 {
		var newIdentifiers []opfID
		for _, id := range file.Identifiers {
			scheme := identifierTypeToScheme(id.Type)
			newIdentifiers = append(newIdentifiers, opfID{
				Text:   id.Value,
				Scheme: scheme,
			})
		}
		pkg.Metadata.Identifiers = newIdentifiers
	}

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

// resolveCoverPath resolves the full path to a file's cover image.
// CoverImageFilename in the model is just a filename, so we need to determine
// the cover directory from the book's filepath.
func resolveCoverPath(book *models.Book, file *models.File) string {
	if file.CoverImageFilename == nil || *file.CoverImageFilename == "" {
		return ""
	}

	// Determine if this is a root-level book (book.Filepath is a file, not a directory)
	isRootLevelBook := false
	if info, err := os.Stat(book.Filepath); err == nil && !info.IsDir() {
		isRootLevelBook = true
	}

	// Determine the cover directory
	var coverDir string
	if isRootLevelBook {
		coverDir = filepath.Dir(book.Filepath)
	} else {
		coverDir = book.Filepath
	}

	return filepath.Join(coverDir, *file.CoverImageFilename)
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

type opfPackage struct {
	XMLName          xml.Name    `xml:"package"`
	Xmlns            string      `xml:"xmlns,attr"`
	Version          string      `xml:"version,attr"`
	UniqueIdentifier string      `xml:"unique-identifier,attr,omitempty"`
	Metadata         opfMetadata `xml:"metadata"`
	Manifest         opfManifest `xml:"manifest"`
	Spine            opfSpine    `xml:"spine"`
	Guide            *opfGuide   `xml:"guide,omitempty"`
}

type opfMetadata struct {
	XMLName     xml.Name     `xml:"metadata"`
	DC          string       `xml:"dc,attr,omitempty"`
	OPF         string       `xml:"opf,attr,omitempty"`
	Titles      []opfTitle   `xml:"title"`
	Creators    []opfCreator `xml:"creator"`
	Identifiers []opfID      `xml:"identifier"`
	Language    string       `xml:"language,omitempty"`
	Publisher   string       `xml:"publisher,omitempty"`
	Date        string       `xml:"date,omitempty"`
	Description string       `xml:"description,omitempty"`
	Rights      string       `xml:"rights,omitempty"`
	Meta        []opfMeta    `xml:"meta"`
	Subjects    []string     `xml:"subject"`
	Contributor *opfCreator  `xml:"contributor,omitempty"`
}

type opfTitle struct {
	Text string `xml:",chardata"`
	ID   string `xml:"id,attr,omitempty"`
}

type opfCreator struct {
	Text   string `xml:",chardata"`
	ID     string `xml:"id,attr,omitempty"`
	Role   string `xml:"role,attr,omitempty"`
	FileAs string `xml:"file-as,attr,omitempty"`
}

type opfID struct {
	Text   string `xml:",chardata"`
	ID     string `xml:"id,attr,omitempty"`
	Scheme string `xml:"scheme,attr,omitempty"`
}

type opfMeta struct {
	Text     string `xml:",chardata"`
	Name     string `xml:"name,attr,omitempty"`
	Content  string `xml:"content,attr,omitempty"`
	ID       string `xml:"id,attr,omitempty"`
	Refines  string `xml:"refines,attr,omitempty"`
	Property string `xml:"property,attr,omitempty"`
}

type opfManifest struct {
	XMLName xml.Name          `xml:"manifest"`
	Items   []opfManifestItem `xml:"item"`
}

type opfManifestItem struct {
	ID        string `xml:"id,attr"`
	Href      string `xml:"href,attr"`
	MediaType string `xml:"media-type,attr"`
}

type opfSpine struct {
	XMLName xml.Name       `xml:"spine"`
	Toc     string         `xml:"toc,attr,omitempty"`
	Items   []opfSpineItem `xml:"itemref"`
}

type opfSpineItem struct {
	IDRef string `xml:"idref,attr"`
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
