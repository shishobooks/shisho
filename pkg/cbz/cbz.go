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
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/fileutils"
	"github.com/shishobooks/shisho/pkg/htmlutil"
	"github.com/shishobooks/shisho/pkg/identifiers"
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
	Summary         string   `xml:"Summary"`
	Web             string   `xml:"Web"`
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

	// Get sorted image files
	imageFiles := getSortedImageFiles(zipReader)

	// Extract image file paths for chapter detection
	imagePaths := make([]string, len(imageFiles))
	for i, f := range imageFiles {
		imagePaths[i] = f.Name
	}

	// Detect chapters from image file paths
	chapters := DetectChapters(imagePaths)

	// Extract cover image, page index, and page count
	coverData, coverMimeType, coverPage, pageCount, err := extractCoverImage(imageFiles, comicInfo)
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

	// Extract genres and tags from ComicInfo
	var genres []string
	var tags []string
	if comicInfo != nil {
		if comicInfo.Genre != "" {
			genres = fileutils.SplitNames(comicInfo.Genre)
		}
		if comicInfo.Tags != "" {
			tags = fileutils.SplitNames(comicInfo.Tags)
		}
	}

	// Extract description from Summary (strip HTML tags for clean display)
	var description string
	if comicInfo != nil && comicInfo.Summary != "" {
		description = htmlutil.StripTags(comicInfo.Summary)
	}

	// Extract URL from Web
	var url string
	if comicInfo != nil && comicInfo.Web != "" {
		url = comicInfo.Web
	}

	// Extract publisher
	var publisher string
	if comicInfo != nil && comicInfo.Publisher != "" {
		publisher = comicInfo.Publisher
	}

	// Extract imprint
	var imprint string
	if comicInfo != nil && comicInfo.Imprint != "" {
		imprint = comicInfo.Imprint
	}

	// Extract release date from Year/Month/Day
	var releaseDate *time.Time
	if comicInfo != nil && comicInfo.Year != "" {
		year, err := strconv.Atoi(comicInfo.Year)
		if err == nil {
			month := 1
			day := 1
			if comicInfo.Month != "" {
				if m, err := strconv.Atoi(comicInfo.Month); err == nil && m >= 1 && m <= 12 {
					month = m
				}
			}
			if comicInfo.Day != "" {
				if d, err := strconv.Atoi(comicInfo.Day); err == nil && d >= 1 && d <= 31 {
					day = d
				}
			}
			t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
			releaseDate = &t
		}
	}

	// If no series number from ComicInfo, try to extract from filename
	if seriesNumber == nil {
		filename := filepath.Base(path)
		if num := extractSeriesNumberFromFilename(filename); num != nil {
			seriesNumber = num
		}
	}

	// Parse GTIN as identifier
	var identifiersList []mediafile.ParsedIdentifier
	if comicInfo != nil && comicInfo.GTIN != "" {
		gtin := strings.TrimSpace(comicInfo.GTIN)
		idType := identifiers.DetectType(gtin, "")
		if idType == identifiers.TypeUnknown {
			// For CBZ, unknown GTIN is stored as "other"
			idType = identifiers.TypeOther
		}
		identifiersList = append(identifiersList, mediafile.ParsedIdentifier{
			Type:  string(idType),
			Value: gtin,
		})
	}

	return &mediafile.ParsedMetadata{
		Title:         title,
		Authors:       authors,
		Series:        series,
		SeriesNumber:  seriesNumber,
		Genres:        genres,
		Tags:          tags,
		Description:   description,
		Publisher:     publisher,
		Imprint:       imprint,
		URL:           url,
		ReleaseDate:   releaseDate,
		CoverMimeType: coverMimeType,
		CoverData:     coverData,
		CoverPage:     coverPage,
		PageCount:     pageCount,
		DataSource:    models.DataSourceCBZMetadata,
		Identifiers:   identifiersList,
		Chapters:      chapters,
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

// getSortedImageFiles returns a sorted list of image files from a zip reader.
func getSortedImageFiles(zipReader *zip.Reader) []*zip.File {
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

	return imageFiles
}

// extractCoverImage extracts the cover image data and returns the page index and total page count.
// Returns coverData, mimeType, pageIndex, pageCount, error.
func extractCoverImage(imageFiles []*zip.File, comicInfo *ComicInfo) ([]byte, string, *int, *int, error) {
	// Calculate page count from actual image files
	pageCount := len(imageFiles)

	if len(imageFiles) == 0 {
		return nil, "", nil, nil, nil
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
		return nil, "", nil, nil, errors.WithStack(err)
	}
	defer r.Close()

	coverData, err := io.ReadAll(r)
	if err != nil {
		return nil, "", nil, nil, errors.WithStack(err)
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

	return coverData, mimeType, coverPageIndex, &pageCount, nil
}

func splitCreators(creators string) []string {
	return fileutils.SplitNames(creators)
}

// cbzParensRE matches parenthesized metadata sections like (2020), (Digital), (group).
var cbzParensRE = regexp.MustCompile(`\([^)]*\)`)

func extractSeriesNumberFromFilename(filename string) *float64 {
	// Remove extension for processing
	nameWithoutExt := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Strip parenthesized metadata (year, quality, group) before matching volume
	nameWithoutExt = cbzParensRE.ReplaceAllString(nameWithoutExt, "")
	nameWithoutExt = strings.TrimSpace(nameWithoutExt)

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
