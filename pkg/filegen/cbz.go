package filegen

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/shishobooks/shisho/pkg/kepub"
	"github.com/shishobooks/shisho/pkg/models"
)

// CBZGenerator generates CBZ comic book files with modified metadata.
type CBZGenerator struct{}

// SupportedType returns the file type this generator handles.
func (g *CBZGenerator) SupportedType() string {
	return models.FileTypeCBZ
}

// Generate creates a modified CBZ at destPath with updated metadata.
// Images are processed to optimize for e-readers (resized, grayscale optimization).
func (g *CBZGenerator) Generate(ctx context.Context, srcPath, destPath string, book *models.Book, file *models.File) error {
	// Open source CBZ
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return NewGenerationError(models.FileTypeCBZ, err, "failed to open source file")
	}
	defer srcFile.Close()

	srcStat, err := srcFile.Stat()
	if err != nil {
		return NewGenerationError(models.FileTypeCBZ, err, "failed to stat source file")
	}

	srcZip, err := zip.NewReader(srcFile, srcStat.Size())
	if err != nil {
		return NewGenerationError(models.FileTypeCBZ, err, "failed to read source CBZ as zip")
	}

	// Find existing ComicInfo.xml and separate image files from other files
	var existingComicInfo *cbzComicInfo
	var imageFiles []*zip.File
	var otherFiles []*zip.File

	for _, f := range srcZip.File {
		lowerName := strings.ToLower(f.Name)
		if lowerName == "comicinfo.xml" {
			existingComicInfo, err = parseComicInfoFromZip(f)
			if err != nil {
				existingComicInfo = nil
			}
			otherFiles = append(otherFiles, f)
		} else if kepub.IsImageFile(f.Name) && !strings.HasPrefix(filepath.Base(f.Name), ".") {
			imageFiles = append(imageFiles, f)
		} else {
			otherFiles = append(otherFiles, f)
		}
	}

	// Process images in parallel for better performance
	type processedFile struct {
		name string
		data []byte
		err  error
	}
	processedImages := make([]processedFile, len(imageFiles))

	// Use a worker pool with NumCPU workers
	numWorkers := runtime.NumCPU()
	if numWorkers > len(imageFiles) {
		numWorkers = len(imageFiles)
	}
	if numWorkers == 0 {
		numWorkers = 1
	}

	var wg sync.WaitGroup
	jobs := make(chan int, len(imageFiles))

	// Start workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				// Check for context cancellation
				select {
				case <-ctx.Done():
					processedImages[i] = processedFile{err: ctx.Err()}
					continue
				default:
				}

				imgFile := imageFiles[i]
				data, err := readCBZZipFile(imgFile)
				if err != nil {
					processedImages[i] = processedFile{err: fmt.Errorf("failed to read image %s: %w", imgFile.Name, err)}
					continue
				}

				// Process image for e-reader optimization
				ext := strings.ToLower(filepath.Ext(imgFile.Name))
				processed := kepub.ProcessImageForEreader(data, ext)

				// Determine the new filename (may change extension if PNG→JPEG)
				newName := imgFile.Name
				if processed.Ext != ext {
					newName = strings.TrimSuffix(imgFile.Name, filepath.Ext(imgFile.Name)) + processed.Ext
				}

				processedImages[i] = processedFile{
					name: newName,
					data: processed.Data,
				}
			}
		}()
	}

	// Send jobs
	for i := range imageFiles {
		jobs <- i
	}
	close(jobs)

	// Wait for all workers to complete
	wg.Wait()

	// Check for errors
	for i, pf := range processedImages {
		if pf.err != nil {
			if errors.Is(pf.err, context.Canceled) || errors.Is(pf.err, context.DeadlineExceeded) {
				return NewGenerationError(models.FileTypeCBZ, pf.err, "generation cancelled")
			}
			return NewGenerationError(models.FileTypeCBZ, pf.err, fmt.Sprintf("failed to process image %d", i+1))
		}
	}

	// Create temporary output file
	tmpPath := destPath + ".tmp"
	destFile, err := os.Create(tmpPath)
	if err != nil {
		return NewGenerationError(models.FileTypeCBZ, err, "failed to create destination file")
	}
	defer func() {
		destFile.Close()
		os.Remove(tmpPath) // Clean up temp file if we don't rename it
	}()

	destZip := zip.NewWriter(destFile)

	// Prepare the modified ComicInfo
	comicInfo := modifyCBZComicInfo(existingComicInfo, book, file)

	// Track if we need to add ComicInfo.xml (if it didn't exist)
	comicInfoWritten := false

	// Process non-image files (including existing ComicInfo.xml)
	for _, srcZipFile := range otherFiles {
		var destFileContent []byte

		if strings.ToLower(srcZipFile.Name) == "comicinfo.xml" {
			// Write modified ComicInfo.xml
			destFileContent, err = marshalComicInfo(comicInfo)
			if err != nil {
				return NewGenerationError(models.FileTypeCBZ, err, "failed to marshal ComicInfo.xml")
			}
			comicInfoWritten = true
		} else {
			// Copy file unchanged
			destFileContent, err = readCBZZipFile(srcZipFile)
			if err != nil {
				return NewGenerationError(models.FileTypeCBZ, err, "failed to read file from source CBZ")
			}
		}

		// Write to destination
		destZipFile, err := destZip.CreateHeader(&zip.FileHeader{
			Name:   srcZipFile.Name,
			Method: srcZipFile.Method,
		})
		if err != nil {
			return NewGenerationError(models.FileTypeCBZ, err, "failed to create file in destination CBZ")
		}

		if _, err := destZipFile.Write(destFileContent); err != nil {
			return NewGenerationError(models.FileTypeCBZ, err, "failed to write file to destination CBZ")
		}
	}

	// Write processed images
	for _, pf := range processedImages {
		destZipFile, err := destZip.Create(pf.name)
		if err != nil {
			return NewGenerationError(models.FileTypeCBZ, err, "failed to create image file in destination CBZ")
		}

		if _, err := destZipFile.Write(pf.data); err != nil {
			return NewGenerationError(models.FileTypeCBZ, err, "failed to write image file to destination CBZ")
		}
	}

	// If no ComicInfo.xml existed, add one
	if !comicInfoWritten {
		destFileContent, err := marshalComicInfo(comicInfo)
		if err != nil {
			return NewGenerationError(models.FileTypeCBZ, err, "failed to marshal new ComicInfo.xml")
		}

		destZipFile, err := destZip.Create("ComicInfo.xml")
		if err != nil {
			return NewGenerationError(models.FileTypeCBZ, err, "failed to create ComicInfo.xml in destination CBZ")
		}

		if _, err := destZipFile.Write(destFileContent); err != nil {
			return NewGenerationError(models.FileTypeCBZ, err, "failed to write ComicInfo.xml to destination CBZ")
		}
	}

	// Close the zip writer
	if err := destZip.Close(); err != nil {
		return NewGenerationError(models.FileTypeCBZ, err, "failed to finalize destination CBZ")
	}

	// Close the file before renaming
	if err := destFile.Close(); err != nil {
		return NewGenerationError(models.FileTypeCBZ, err, "failed to close destination file")
	}

	// Atomic rename
	if err := os.Rename(tmpPath, destPath); err != nil {
		return NewGenerationError(models.FileTypeCBZ, err, "failed to finalize destination file")
	}

	return nil
}

// cbzComicInfo represents ComicInfo.xml structure for CBZ generation.
// Uses pointers for optional string fields to distinguish empty from omitted.
type cbzComicInfo struct {
	XMLName         xml.Name  `xml:"ComicInfo"`
	Title           string    `xml:"Title,omitempty"`
	Series          string    `xml:"Series,omitempty"`
	Number          string    `xml:"Number,omitempty"`
	Volume          string    `xml:"Volume,omitempty"`
	Summary         string    `xml:"Summary,omitempty"`
	Year            string    `xml:"Year,omitempty"`
	Month           string    `xml:"Month,omitempty"`
	Day             string    `xml:"Day,omitempty"`
	Writer          string    `xml:"Writer,omitempty"`
	Penciller       string    `xml:"Penciller,omitempty"`
	Inker           string    `xml:"Inker,omitempty"`
	Colorist        string    `xml:"Colorist,omitempty"`
	Letterer        string    `xml:"Letterer,omitempty"`
	CoverArtist     string    `xml:"CoverArtist,omitempty"`
	Editor          string    `xml:"Editor,omitempty"`
	Translator      string    `xml:"Translator,omitempty"`
	Publisher       string    `xml:"Publisher,omitempty"`
	Imprint         string    `xml:"Imprint,omitempty"`
	Genre           string    `xml:"Genre,omitempty"`
	Tags            string    `xml:"Tags,omitempty"`
	Web             string    `xml:"Web,omitempty"`
	Characters      string    `xml:"Characters,omitempty"`
	Teams           string    `xml:"Teams,omitempty"`
	Locations       string    `xml:"Locations,omitempty"`
	StoryArc        string    `xml:"StoryArc,omitempty"`
	AgeRating       string    `xml:"AgeRating,omitempty"`
	CommunityRating string    `xml:"CommunityRating,omitempty"`
	PageCount       string    `xml:"PageCount,omitempty"`
	LanguageISO     string    `xml:"LanguageISO,omitempty"`
	Format          string    `xml:"Format,omitempty"`
	BlackAndWhite   string    `xml:"BlackAndWhite,omitempty"`
	Manga           string    `xml:"Manga,omitempty"`
	GTIN            string    `xml:"GTIN,omitempty"`
	Pages           *cbzPages `xml:"Pages,omitempty"`
}

type cbzPages struct {
	Page []cbzPageInfo `xml:"Page"`
}

type cbzPageInfo struct {
	Image       string `xml:"Image,attr,omitempty"`
	Type        string `xml:"Type,attr,omitempty"`
	DoublePage  string `xml:"DoublePage,attr,omitempty"`
	ImageSize   string `xml:"ImageSize,attr,omitempty"`
	ImageWidth  string `xml:"ImageWidth,attr,omitempty"`
	ImageHeight string `xml:"ImageHeight,attr,omitempty"`
}

// parseComicInfoFromZip reads and parses ComicInfo.xml from a zip file entry.
func parseComicInfoFromZip(f *zip.File) (*cbzComicInfo, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var comicInfo cbzComicInfo
	if err := xml.Unmarshal(data, &comicInfo); err != nil {
		return nil, err
	}

	return &comicInfo, nil
}

// modifyCBZComicInfo updates a ComicInfo structure with book metadata.
// Only modifies tracked fields; preserves all others.
func modifyCBZComicInfo(existing *cbzComicInfo, book *models.Book, file *models.File) *cbzComicInfo {
	// Start with existing or empty struct
	var comicInfo cbzComicInfo
	if existing != nil {
		comicInfo = *existing
	}

	// Update title - prefer file.Name over book.Title
	title := book.Title
	if file != nil && file.Name != nil && *file.Name != "" {
		title = *file.Name
	}
	comicInfo.Title = title

	// Update series
	if len(book.BookSeries) > 0 {
		// Sort by sort order and use primary series
		series := make([]*models.BookSeries, len(book.BookSeries))
		copy(series, book.BookSeries)
		sort.Slice(series, func(i, j int) bool {
			return series[i].SortOrder < series[j].SortOrder
		})

		first := series[0]
		if first.Series != nil {
			comicInfo.Series = first.Series.Name
		}
		if first.SeriesNumber != nil {
			comicInfo.Number = formatCBZNumber(*first.SeriesNumber)
		} else {
			comicInfo.Number = ""
		}
	} else {
		comicInfo.Series = ""
		comicInfo.Number = ""
	}

	// Update author roles from book's authors
	// First, collect authors by role
	authorsByRole := make(map[string][]string)
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
			role := ""
			if a.Role != nil {
				role = *a.Role
			}
			authorsByRole[role] = append(authorsByRole[role], a.Person.Name)
		}
	}

	// Map roles to ComicInfo fields
	comicInfo.Writer = strings.Join(authorsByRole[models.AuthorRoleWriter], ", ")
	comicInfo.Penciller = strings.Join(authorsByRole[models.AuthorRolePenciller], ", ")
	comicInfo.Inker = strings.Join(authorsByRole[models.AuthorRoleInker], ", ")
	comicInfo.Colorist = strings.Join(authorsByRole[models.AuthorRoleColorist], ", ")
	comicInfo.Letterer = strings.Join(authorsByRole[models.AuthorRoleLetterer], ", ")
	comicInfo.CoverArtist = strings.Join(authorsByRole[models.AuthorRoleCoverArtist], ", ")
	comicInfo.Editor = strings.Join(authorsByRole[models.AuthorRoleEditor], ", ")
	comicInfo.Translator = strings.Join(authorsByRole[models.AuthorRoleTranslator], ", ")

	// Authors with no role go to Writer field (if no explicit writers)
	if comicInfo.Writer == "" && len(authorsByRole[""]) > 0 {
		comicInfo.Writer = strings.Join(authorsByRole[""], ", ")
	}

	// Update genres (comma-separated) or preserve existing
	if len(book.BookGenres) > 0 {
		var genreNames []string
		for _, bg := range book.BookGenres {
			if bg.Genre != nil {
				genreNames = append(genreNames, bg.Genre.Name)
			}
		}
		comicInfo.Genre = strings.Join(genreNames, ", ")
	}
	// If no book genres, preserve existing Genre from comicInfo (already copied from existing)

	// Update tags (comma-separated) or preserve existing
	if len(book.BookTags) > 0 {
		var tagNames []string
		for _, bt := range book.BookTags {
			if bt.Tag != nil {
				tagNames = append(tagNames, bt.Tag.Name)
			}
		}
		comicInfo.Tags = strings.Join(tagNames, ", ")
	}
	// If no book tags, preserve existing Tags from comicInfo (already copied from existing)

	// Update description (Summary in ComicInfo.xml)
	if book.Description != nil && *book.Description != "" {
		comicInfo.Summary = *book.Description
	}

	// Update URL (Web in ComicInfo.xml)
	if file.URL != nil && *file.URL != "" {
		comicInfo.Web = *file.URL
	}

	// Update publisher
	if file.Publisher != nil {
		comicInfo.Publisher = file.Publisher.Name
	}

	// Update imprint
	if file.Imprint != nil {
		comicInfo.Imprint = file.Imprint.Name
	}

	// Update release date (Year, Month, Day)
	if file.ReleaseDate != nil {
		comicInfo.Year = strconv.Itoa(file.ReleaseDate.Year())
		comicInfo.Month = strconv.Itoa(int(file.ReleaseDate.Month()))
		comicInfo.Day = strconv.Itoa(file.ReleaseDate.Day())
	}

	// Update cover page in Pages section
	if file.CoverPage != nil {
		updateCoverPage(&comicInfo, *file.CoverPage)
	}

	// Write GTIN from file identifiers (priority: ISBN-13 > ISBN-10 > Other > ASIN)
	if file != nil && len(file.Identifiers) > 0 {
		if gtin := selectGTIN(file.Identifiers); gtin != "" {
			comicInfo.GTIN = gtin
		}
	}

	return &comicInfo
}

// updateCoverPage sets the FrontCover type on the specified page index.
func updateCoverPage(comicInfo *cbzComicInfo, coverPageIndex int) {
	if comicInfo.Pages == nil {
		// No Pages section exists - create one with the cover page
		comicInfo.Pages = &cbzPages{
			Page: []cbzPageInfo{
				{Image: strconv.Itoa(coverPageIndex), Type: "FrontCover"},
			},
		}
		return
	}

	// Clear existing FrontCover types
	for i := range comicInfo.Pages.Page {
		if strings.ToLower(comicInfo.Pages.Page[i].Type) == "frontcover" {
			comicInfo.Pages.Page[i].Type = ""
		}
	}

	// Find the page with matching index and set FrontCover
	found := false
	for i := range comicInfo.Pages.Page {
		pageNum, err := strconv.Atoi(comicInfo.Pages.Page[i].Image)
		if err == nil && pageNum == coverPageIndex {
			comicInfo.Pages.Page[i].Type = "FrontCover"
			found = true
			break
		}
	}

	// If not found, add the page entry
	if !found {
		comicInfo.Pages.Page = append(comicInfo.Pages.Page, cbzPageInfo{
			Image: strconv.Itoa(coverPageIndex),
			Type:  "FrontCover",
		})
	}
}

// marshalComicInfo serializes ComicInfo to XML with proper formatting.
func marshalComicInfo(comicInfo *cbzComicInfo) ([]byte, error) {
	output, err := xml.MarshalIndent(comicInfo, "", "  ")
	if err != nil {
		return nil, err
	}

	// Add XML declaration
	result := append([]byte(xml.Header), output...)
	return result, nil
}

// readCBZZipFile reads the contents of a zip file entry.
func readCBZZipFile(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}

// formatCBZNumber formats a float64 for series number.
// Whole numbers display without decimal (e.g., "1"), decimals are preserved (e.g., "1.5").
func formatCBZNumber(f float64) string {
	if f == math.Floor(f) {
		return strconv.Itoa(int(f))
	}
	return fmt.Sprintf("%g", f)
}

// selectGTIN selects the best identifier to use as GTIN (priority: ISBN-13 > ISBN-10 > Other > ASIN).
func selectGTIN(identifiers []*models.FileIdentifier) string {
	priorityOrder := []string{"isbn_13", "isbn_10", "other", "asin"}
	for _, priority := range priorityOrder {
		for _, id := range identifiers {
			if id.Type == priority {
				return id.Value
			}
		}
	}
	return ""
}
