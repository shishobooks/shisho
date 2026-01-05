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
	if file.CoverImagePath != nil && *file.CoverImagePath != "" {
		// Resolve the full cover path from the book's directory
		// CoverImagePath is just a filename, we need to find the cover directory
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
			destFileContent, err = modifyOPF(srcZipFile, book, coverInfo, newCoverMimeType)
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
func modifyOPF(opfFile *zip.File, book *models.Book, coverInfo *coverImageInfo, newCoverMimeType string) ([]byte, error) {
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

	// Update title
	if len(pkg.Metadata.Titles) > 0 {
		pkg.Metadata.Titles[0].Text = book.Title
	} else {
		pkg.Metadata.Titles = []opfTitle{{Text: book.Title}}
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

	// Update series - using Calibre meta tags
	// First, remove existing series meta tags
	var filteredMetas []opfMeta
	for _, meta := range pkg.Metadata.Meta {
		if meta.Name != "calibre:series" && meta.Name != "calibre:series_index" {
			filteredMetas = append(filteredMetas, meta)
		}
	}

	// Add series info sorted by sort order
	if len(book.BookSeries) > 0 {
		series := make([]*models.BookSeries, len(book.BookSeries))
		copy(series, book.BookSeries)
		sort.Slice(series, func(i, j int) bool {
			return series[i].SortOrder < series[j].SortOrder
		})

		// For primary series (first one), use calibre:series
		first := series[0]
		if first.Series != nil {
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
		}
	}
	pkg.Metadata.Meta = filteredMetas

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
// CoverImagePath in the model is just a filename, so we need to determine
// the cover directory from the book's filepath.
func resolveCoverPath(book *models.Book, file *models.File) string {
	if file.CoverImagePath == nil || *file.CoverImagePath == "" {
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

	return filepath.Join(coverDir, *file.CoverImagePath)
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
