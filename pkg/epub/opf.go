package epub

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/htmlutil"
	"github.com/shishobooks/shisho/pkg/identifiers"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
)

type OPF struct {
	Title         string
	Subtitle      string
	Authors       []mediafile.ParsedAuthor
	Series        string
	SeriesNumber  *float64
	Genres        []string
	Tags          []string
	Description   string
	Publisher     string
	Imprint       string
	URL           string
	ReleaseDate   *time.Time
	CoverFilepath string
	CoverMimeType string
	CoverData     []byte
	Identifiers   []mediafile.ParsedIdentifier
	Chapters      []mediafile.ParsedChapter
}

type Package struct {
	XMLName          xml.Name `xml:"package"`
	Text             string   `xml:",chardata"`
	Xmlns            string   `xml:"xmlns,attr"`
	Version          string   `xml:"version,attr"`
	UniqueIdentifier string   `xml:"unique-identifier,attr"`
	Metadata         struct {
		Text    string `xml:",chardata"`
		Opf     string `xml:"opf,attr"`
		Dc      string `xml:"dc,attr"`
		Dcterms string `xml:"dcterms,attr"`
		Xsi     string `xml:"xsi,attr"`
		Calibre string `xml:"calibre,attr"`
		Title   []struct {
			Text string `xml:",chardata"`
			ID   string `xml:"id,attr"`
		} `xml:"title"`
		Creator []struct {
			Text   string `xml:",chardata"`
			ID     string `xml:"id,attr"`
			Role   string `xml:"role,attr"`
			FileAs string `xml:"file-as,attr"`
		} `xml:"creator"`
		Contributor struct {
			Text string `xml:",chardata"`
			Role string `xml:"role,attr"`
		} `xml:"contributor"`
		Description string   `xml:"description"`
		Subject     []string `xml:"subject"`
		Publisher   string   `xml:"publisher"`
		Identifier  []struct {
			Text   string `xml:",chardata"`
			ID     string `xml:"id,attr"`
			Scheme string `xml:"scheme,attr"`
		} `xml:"identifier"`
		Date     string   `xml:"date"`
		Relation []string `xml:"relation"`
		Source   []string `xml:"source"`
		Rights   string   `xml:"rights"`
		Language string   `xml:"language"`
		Meta     []struct {
			Text     string `xml:",chardata"`
			Name     string `xml:"name,attr"`
			Content  string `xml:"content,attr"`
			Refines  string `xml:"refines,attr"`
			Property string `xml:"property,attr"`
		} `xml:"meta"`
	} `xml:"metadata"`
	Manifest struct {
		Text string `xml:",chardata"`
		Item []struct {
			Text       string `xml:",chardata"`
			ID         string `xml:"id,attr"`
			Href       string `xml:"href,attr"`
			MediaType  string `xml:"media-type,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		Text    string `xml:",chardata"`
		Toc     string `xml:"toc,attr"`
		Itemref []struct {
			Text  string `xml:",chardata"`
			Idref string `xml:"idref,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
	Guide struct {
		Text      string `xml:",chardata"`
		Reference []struct {
			Text  string `xml:",chardata"`
			Type  string `xml:"type,attr"`
			Href  string `xml:"href,attr"`
			Title string `xml:"title,attr"`
		} `xml:"reference"`
	} `xml:"guide"`
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

	// Open the existing archive to go through each of the pages.
	zipReader, err := zip.NewReader(f, size)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Go through all files in the existing archive save the page information.
	var result *ParseOPFResult
	for _, file := range zipReader.File {
		ext := filepath.Ext(file.Name)
		if ext == ".opf" {
			r, err := file.Open()
			if err != nil {
				return nil, errors.WithStack(err)
			}
			result, err = ParseOPF(file.Name, r)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			break
		}
	}

	if result == nil {
		return nil, errors.New("no opf file found")
	}

	opf := result.OPF

	if opf.CoverFilepath != "" {
		for _, file := range zipReader.File {
			if file.Name == opf.CoverFilepath {
				r, err := file.Open()
				if err != nil {
					return nil, errors.WithStack(err)
				}
				b, err := io.ReadAll(r)
				if err != nil {
					return nil, errors.WithStack(err)
				}
				opf.CoverData = b
			}
		}
	}

	// Try to find and parse chapters from nav document or NCX
	navHref := findNavDocumentHref(result.Package, result.BasePath)
	ncxHref := findNCXHref(result.Package, result.BasePath)

	for _, file := range zipReader.File {
		if navHref != "" && file.Name == navHref {
			r, err := file.Open()
			if err == nil {
				chapters, _ := parseNavDocument(r)
				r.Close()
				if len(chapters) > 0 {
					opf.Chapters = chapters
					break
				}
			}
		}
		if ncxHref != "" && file.Name == ncxHref && len(opf.Chapters) == 0 {
			r, err := file.Open()
			if err == nil {
				chapters, _ := parseNCX(r)
				r.Close()
				opf.Chapters = chapters
			}
		}
	}

	return &mediafile.ParsedMetadata{
		Title:         opf.Title,
		Subtitle:      opf.Subtitle,
		Authors:       opf.Authors,
		Series:        opf.Series,
		SeriesNumber:  opf.SeriesNumber,
		Genres:        opf.Genres,
		Tags:          opf.Tags,
		Description:   opf.Description,
		Publisher:     opf.Publisher,
		Imprint:       opf.Imprint,
		URL:           opf.URL,
		ReleaseDate:   opf.ReleaseDate,
		CoverMimeType: opf.CoverMimeType,
		CoverData:     opf.CoverData,
		DataSource:    models.DataSourceEPUBMetadata,
		Identifiers:   opf.Identifiers,
		Chapters:      opf.Chapters,
	}, nil
}

// ParseOPFResult contains the parsed OPF data along with the raw package and base path
// needed for resolving relative paths to other files in the EPUB.
type ParseOPFResult struct {
	OPF      *OPF
	Package  *Package
	BasePath string
}

func ParseOPF(filename string, r io.ReadCloser) (*ParseOPFResult, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	pkg := &Package{}
	err = xml.Unmarshal(b, pkg)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Determine the base path because all files are referenced from the location of the OPF file. If basePath is `.`,
	// that means it's at the root of the EPUB and should not be included. But if it's something else, we need to tack
	// on a `/` since we'll be adding it as a prefix to all file paths.
	basePath := filepath.Dir(filename)
	if basePath == "." {
		basePath = ""
	} else {
		basePath += "/"
	}

	// Parse out metadata into a more lookup-friendly structure.
	metaProperties := map[string]map[string]string{}
	metaContent := map[string]string{}
	for _, m := range pkg.Metadata.Meta {
		if m.Refines != "" {
			key := strings.ReplaceAll(m.Refines, "#", "")
			if _, ok := metaProperties[key]; !ok {
				metaProperties[key] = map[string]string{}
			}
			metaProperties[key][m.Property] = m.Text
		} else if m.Content != "" {
			metaContent[m.Name] = m.Content
		}
	}

	// Parse out the main title and subtitle of the book.
	title := ""
	subtitle := ""
	if len(pkg.Metadata.Title) == 1 {
		title = pkg.Metadata.Title[0].Text
	} else if len(pkg.Metadata.Title) > 1 {
		for _, t := range pkg.Metadata.Title {
			titleType := ""
			if t.ID != "" && metaProperties[t.ID] != nil {
				titleType = metaProperties[t.ID]["title-type"]
			}
			// Check for main title
			if titleType == "main" || t.ID == "title-main" {
				title = t.Text
			}
			// Check for subtitle - either by title-type property or by id
			if titleType == "subtitle" || t.ID == "subtitle" {
				subtitle = t.Text
			}
		}
		// If we didn't find a main title, use the first one
		if title == "" && len(pkg.Metadata.Title) > 0 {
			title = pkg.Metadata.Title[0].Text
		}
	}

	authors := []mediafile.ParsedAuthor{}
	for _, creator := range pkg.Metadata.Creator {
		role := creator.Role
		if role == "" && creator.ID != "" && metaProperties[creator.ID] != nil {
			role = metaProperties[creator.ID]["role"]
		}
		if role == "aut" || len(pkg.Metadata.Creator) == 1 {
			// EPUB authors have no specific role (generic author)
			authors = append(authors, mediafile.ParsedAuthor{Name: creator.Text, Role: ""})
		}
	}

	coverFilepath := ""
	coverMimeType := ""
	if metaContent["cover"] != "" {
		for _, item := range pkg.Manifest.Item {
			if item.ID == metaContent["cover"] {
				coverFilepath = basePath + item.Href
				coverMimeType = item.MediaType
			}
		}
	}

	// Parse series information from calibre meta tags
	series := metaContent["calibre:series"]
	var seriesNumber *float64
	if seriesIndexStr := metaContent["calibre:series_index"]; seriesIndexStr != "" {
		if num, err := strconv.ParseFloat(seriesIndexStr, 64); err == nil {
			seriesNumber = &num
		}
	}

	// Parse genres from dc:subject elements
	var genres []string
	for _, subject := range pkg.Metadata.Subject {
		subject = strings.TrimSpace(subject)
		if subject != "" {
			genres = append(genres, subject)
		}
	}

	// Parse tags from calibre:tags meta (comma-separated)
	var tags []string
	if calibreTags := metaContent["calibre:tags"]; calibreTags != "" {
		for _, tag := range strings.Split(calibreTags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}

	// Extract description (strip HTML tags for clean display)
	description := htmlutil.StripTags(pkg.Metadata.Description)

	// Extract publisher
	publisher := pkg.Metadata.Publisher

	// Extract release date from dc:date
	var releaseDate *time.Time
	if pkg.Metadata.Date != "" {
		// Try various date formats
		formats := []string{
			"2006-01-02",
			"2006-01-02T15:04:05Z",
			"2006-01-02T15:04:05-07:00",
			"2006",
		}
		for _, format := range formats {
			if t, err := time.Parse(format, pkg.Metadata.Date); err == nil {
				releaseDate = &t
				break
			}
		}
	}

	// Extract imprint from meta tags
	var imprint string
	for _, m := range pkg.Metadata.Meta {
		if m.Property == "ibooks:imprint" || m.Name == "imprint" {
			imprint = m.Text
			if imprint == "" {
				imprint = m.Content
			}
			break
		}
	}

	// Extract URL from dc:relation or dc:source
	var url string
	for _, rel := range pkg.Metadata.Relation {
		if strings.HasPrefix(rel, "http://") || strings.HasPrefix(rel, "https://") {
			url = rel
			break
		}
	}
	if url == "" {
		for _, src := range pkg.Metadata.Source {
			if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
				url = src
				break
			}
		}
	}

	// Parse identifiers from dc:identifier elements
	var identifiersList []mediafile.ParsedIdentifier
	for _, identifier := range pkg.Metadata.Identifier {
		value := strings.TrimSpace(identifier.Text)
		if value == "" {
			continue
		}
		idType := identifiers.DetectType(value, identifier.Scheme)
		if idType == identifiers.TypeUnknown {
			// Skip unknown identifier types for EPUB
			continue
		}
		identifiersList = append(identifiersList, mediafile.ParsedIdentifier{
			Type:  string(idType),
			Value: value,
		})
	}

	return &ParseOPFResult{
		OPF: &OPF{
			Title:         title,
			Subtitle:      subtitle,
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
			CoverFilepath: coverFilepath,
			CoverMimeType: coverMimeType,
			Identifiers:   identifiersList,
		},
		Package:  pkg,
		BasePath: basePath,
	}, nil
}
