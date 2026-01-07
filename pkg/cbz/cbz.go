package cbz

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
)

type ComicInfo struct {
	XMLName         xml.Name `xml:"ComicInfo"`
	Title           string   `xml:"Title"`
	Series          string   `xml:"Series"`
	Number          string   `xml:"Number"`
	Volume          string   `xml:"Volume"`
	Year            string   `xml:"Year"`
	Month           string   `xml:"Month"`
	Day             string   `xml:"Day"`
	Writer          string   `xml:"Writer"`
	Penciller       string   `xml:"Penciller"`
	Inker           string   `xml:"Inker"`
	Colorist        string   `xml:"Colorist"`
	Letterer        string   `xml:"Letterer"`
	CoverArtist     string   `xml:"CoverArtist"`
	Editor          string   `xml:"Editor"`
	Translator      string   `xml:"Translator"`
	Publisher       string   `xml:"Publisher"`
	Imprint         string   `xml:"Imprint"`
	Genre           string   `xml:"Genre"`
	Tags            string   `xml:"Tags"`
	Characters      string   `xml:"Characters"`
	Teams           string   `xml:"Teams"`
	Locations       string   `xml:"Locations"`
	StoryArc        string   `xml:"StoryArc"`
	AgeRating       string   `xml:"AgeRating"`
	CommunityRating string   `xml:"CommunityRating"`
	PageCount       string   `xml:"PageCount"`
	LanguageISO     string   `xml:"LanguageISO"`
	Format          string   `xml:"Format"`
	BlackAndWhite   string   `xml:"BlackAndWhite"`
	Manga           string   `xml:"Manga"`
	GTIN            string   `xml:"GTIN"`
	Pages           struct {
		Page []ComicPageInfo `xml:"Page"`
	} `xml:"Pages"`
}

type ComicPageInfo struct {
	Image       string `xml:"Image,attr"`
	Type        string `xml:"Type,attr"`
	DoublePage  string `xml:"DoublePage,attr"`
	ImageSize   string `xml:"ImageSize,attr"`
	ImageWidth  string `xml:"ImageWidth,attr"`
	ImageHeight string `xml:"ImageHeight,attr"`
}

func Parse(path string) (*mediafile.ParsedMetadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer f.Close()

	stats, err := f.Stat()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	size := stats.Size()

	zipReader, err := zip.NewReader(f, size)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Parse ComicInfo.xml if it exists
	var comicInfo *ComicInfo
	for _, file := range zipReader.File {
		if strings.ToLower(file.Name) == "comicinfo.xml" {
			r, err := file.Open()
			if err != nil {
				return nil, errors.WithStack(err)
			}
			comicInfo, err = ParseComicInfo(r)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			break
		}
	}

	// Extract cover image and page index
	coverData, coverMimeType, coverPage, err := extractCoverImage(zipReader, comicInfo)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Build metadata from ComicInfo
	title := ""
	authors := []mediafile.ParsedAuthor{}
	series := ""
	var seriesNumber *float64

	if comicInfo != nil {
		title = comicInfo.Title
		series = comicInfo.Series

		// Use series number from ComicInfo if available
		if comicInfo.Number != "" {
			if num, err := strconv.ParseFloat(comicInfo.Number, 64); err == nil {
				seriesNumber = &num
			}
		}

		// Collect all creator fields with their roles
		// Track seen names per role to avoid duplicates within the same role
		seenByRole := make(map[string]map[string]bool)

		addCreators := func(creatorStr, role string) {
			if creatorStr == "" {
				return
			}
			if seenByRole[role] == nil {
				seenByRole[role] = make(map[string]bool)
			}
			for _, name := range splitCreators(creatorStr) {
				if !seenByRole[role][name] {
					authors = append(authors, mediafile.ParsedAuthor{Name: name, Role: role})
					seenByRole[role][name] = true
				}
			}
		}

		addCreators(comicInfo.Writer, models.AuthorRoleWriter)
		addCreators(comicInfo.Penciller, models.AuthorRolePenciller)
		addCreators(comicInfo.Inker, models.AuthorRoleInker)
		addCreators(comicInfo.Colorist, models.AuthorRoleColorist)
		addCreators(comicInfo.Letterer, models.AuthorRoleLetterer)
		addCreators(comicInfo.CoverArtist, models.AuthorRoleCoverArtist)
		addCreators(comicInfo.Editor, models.AuthorRoleEditor)
		addCreators(comicInfo.Translator, models.AuthorRoleTranslator)
	}

	// If no series number from ComicInfo, try to extract from filename
	if seriesNumber == nil {
		filename := filepath.Base(path)
		if num := extractSeriesNumberFromFilename(filename); num != nil {
			seriesNumber = num
		}
	}

	return &mediafile.ParsedMetadata{
		Title:         title,
		Authors:       authors,
		Series:        series,
		SeriesNumber:  seriesNumber,
		CoverMimeType: coverMimeType,
		CoverData:     coverData,
		CoverPage:     coverPage,
		DataSource:    models.DataSourceCBZMetadata,
	}, nil
}

func ParseComicInfo(r io.ReadCloser) (*ComicInfo, error) {
	defer r.Close()

	b, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	comicInfo := &ComicInfo{}
	err = xml.Unmarshal(b, comicInfo)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return comicInfo, nil
}

// extractCoverImage extracts the cover image data and returns the page index.
// Returns coverData, mimeType, pageIndex, error.
func extractCoverImage(zipReader *zip.Reader, comicInfo *ComicInfo) ([]byte, string, *int, error) {
	// Create a sorted list of all image files
	var imageFiles []*zip.File
	for _, file := range zipReader.File {
		ext := strings.ToLower(filepath.Ext(file.Name))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp" {
			imageFiles = append(imageFiles, file)
		}
	}

	// Sort image files by name to ensure consistent ordering
	sort.Slice(imageFiles, func(i, j int) bool {
		return imageFiles[i].Name < imageFiles[j].Name
	})

	if len(imageFiles) == 0 {
		return nil, "", nil, nil
	}

	var targetFile *zip.File
	var coverPageIndex *int

	// Strategy 1: Look for FrontCover in ComicInfo.xml
	if comicInfo != nil && len(comicInfo.Pages.Page) > 0 {
		for _, page := range comicInfo.Pages.Page {
			if strings.ToLower(page.Type) == "frontcover" {
				// Find the image file corresponding to this page
				pageNum, err := strconv.Atoi(page.Image)
				if err == nil && pageNum >= 0 && pageNum < len(imageFiles) {
					targetFile = imageFiles[pageNum]
					coverPageIndex = &pageNum
					break
				}
			}
		}

		// Strategy 2: Look for InnerCover if no FrontCover found
		if targetFile == nil {
			for _, page := range comicInfo.Pages.Page {
				if strings.ToLower(page.Type) == "innercover" {
					pageNum, err := strconv.Atoi(page.Image)
					if err == nil && pageNum >= 0 && pageNum < len(imageFiles) {
						targetFile = imageFiles[pageNum]
						coverPageIndex = &pageNum
						break
					}
				}
			}
		}
	}

	// Strategy 3: Use the first image file
	if targetFile == nil {
		targetFile = imageFiles[0]
		zero := 0
		coverPageIndex = &zero
	}

	// Extract the cover image data
	r, err := targetFile.Open()
	if err != nil {
		return nil, "", nil, errors.WithStack(err)
	}
	defer r.Close()

	coverData, err := io.ReadAll(r)
	if err != nil {
		return nil, "", nil, errors.WithStack(err)
	}

	// Determine MIME type from extension
	ext := strings.ToLower(filepath.Ext(targetFile.Name))
	mimeType := ""
	switch ext {
	case ".jpg", ".jpeg":
		mimeType = "image/jpeg"
	case ".png":
		mimeType = "image/png"
	case ".gif":
		mimeType = "image/gif"
	case ".webp":
		mimeType = "image/webp"
	}

	return coverData, mimeType, coverPageIndex, nil
}

func splitCreators(creators string) []string {
	if creators == "" {
		return nil
	}

	// Split by comma and clean up whitespace
	parts := strings.Split(creators, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func extractSeriesNumberFromFilename(filename string) *float64 {
	// Remove extension for processing
	nameWithoutExt := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Try patterns: #7, v7, or just 7 at the end
	patterns := []string{
		`(?i)#(\d+(?:\.\d+)?)$`,   // matches #7 or #7.5
		`(?i)v(\d+(?:\.\d+)?)$`,   // matches v7 or v7.5
		`(?i)\s+(\d+(?:\.\d+)?)$`, // matches " 7" or " 7.5"
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(nameWithoutExt); len(matches) >= 2 {
			if num, err := strconv.ParseFloat(matches[1], 64); err == nil {
				return &num
			}
		}
	}

	return nil
}
