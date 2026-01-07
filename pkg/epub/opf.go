package epub

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
)

type OPF struct {
	Title         string
	Authors       []mediafile.ParsedAuthor
	Series        string
	SeriesNumber  *float64
	CoverFilepath string
	CoverMimeType string
	CoverData     []byte
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
		Description string `xml:"description"`
		Publisher   string `xml:"publisher"`
		Identifier  []struct {
			Text   string `xml:",chardata"`
			ID     string `xml:"id,attr"`
			Scheme string `xml:"scheme,attr"`
		} `xml:"identifier"`
		Date     string `xml:"date"`
		Rights   string `xml:"rights"`
		Language string `xml:"language"`
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
			Text      string `xml:",chardata"`
			ID        string `xml:"id,attr"`
			Href      string `xml:"href,attr"`
			MediaType string `xml:"media-type,attr"`
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
	var opf *OPF
	for _, file := range zipReader.File {
		ext := filepath.Ext(file.Name)
		if ext == ".opf" {
			r, err := file.Open()
			if err != nil {
				return nil, errors.WithStack(err)
			}
			opf, err = ParseOPF(file.Name, r)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			break
		}
	}

	if opf == nil {
		return nil, errors.New("no opf file found")
	}

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

	return &mediafile.ParsedMetadata{
		Title:         opf.Title,
		Authors:       opf.Authors,
		Series:        opf.Series,
		SeriesNumber:  opf.SeriesNumber,
		CoverMimeType: opf.CoverMimeType,
		CoverData:     opf.CoverData,
		DataSource:    models.DataSourceEPUBMetadata,
	}, nil
}

func ParseOPF(filename string, r io.ReadCloser) (*OPF, error) {
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

	// Parse out the main title of the book.
	title := ""
	if len(pkg.Metadata.Title) == 1 {
		title = pkg.Metadata.Title[0].Text
	} else if len(pkg.Metadata.Title) > 1 {
		for _, t := range pkg.Metadata.Title {
			if t.ID != "" && metaProperties[t.ID] != nil && metaProperties[t.ID]["title-type"] == "main" {
				title = t.Text
				break
			}
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

	return &OPF{
		Title:         title,
		Authors:       authors,
		Series:        series,
		SeriesNumber:  seriesNumber,
		CoverFilepath: coverFilepath,
		CoverMimeType: coverMimeType,
	}, nil
}
