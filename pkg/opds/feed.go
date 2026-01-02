package opds

import (
	"encoding/xml"
	"time"
)

// OPDS namespaces.
const (
	AtomNS     = "http://www.w3.org/2005/Atom"
	DCNS       = "http://purl.org/dc/terms/"
	OPDSNS     = "http://opds-spec.org/2010/catalog"
	OpenSearch = "http://a9.com/-/spec/opensearch/1.1/"
)

// Link relation types.
const (
	RelSelf        = "self"
	RelStart       = "start"
	RelUp          = "up"
	RelSubsection  = "subsection"
	RelNext        = "next"
	RelPrevious    = "previous"
	RelFirst       = "first"
	RelLast        = "last"
	RelSearch      = "search"
	RelAcquisition = "http://opds-spec.org/acquisition"
	RelImage       = "http://opds-spec.org/image"
	RelThumbnail   = "http://opds-spec.org/image/thumbnail"
)

// MIME types.
const (
	MimeTypeAtom        = "application/atom+xml"
	MimeTypeNavigation  = "application/atom+xml;profile=opds-catalog;kind=navigation"
	MimeTypeAcquisition = "application/atom+xml;profile=opds-catalog;kind=acquisition"
	MimeTypeOpenSearch  = "application/opensearchdescription+xml"
	MimeTypeEPUB        = "application/epub+zip"
	MimeTypeCBZ         = "application/vnd.comicbook+zip"
	MimeTypeM4B         = "audio/mp4"
	MimeTypeJPEG        = "image/jpeg"
	MimeTypePNG         = "image/png"
	MimeTypeWebP        = "image/webp"
)

// Feed represents an OPDS Atom feed.
type Feed struct {
	XMLName   xml.Name  `xml:"feed"`
	Xmlns     string    `xml:"xmlns,attr"`
	XmlnsDC   string    `xml:"xmlns:dc,attr,omitempty"`
	XmlnsOPDS string    `xml:"xmlns:opds,attr,omitempty"`
	ID        string    `xml:"id"`
	Title     string    `xml:"title"`
	Updated   time.Time `xml:"updated"`
	Author    *Author   `xml:"author,omitempty"`
	Links     []Link    `xml:"link"`
	Entries   []Entry   `xml:"entry"`
}

// NewFeed creates a new OPDS feed with default namespaces.
func NewFeed(id, title string) *Feed {
	return &Feed{
		Xmlns:     AtomNS,
		XmlnsDC:   DCNS,
		XmlnsOPDS: OPDSNS,
		ID:        id,
		Title:     title,
		Updated:   time.Now().UTC(),
		Links:     []Link{},
		Entries:   []Entry{},
	}
}

// AddLink adds a link to the feed.
func (f *Feed) AddLink(rel, href, linkType string) {
	f.Links = append(f.Links, Link{
		Rel:  rel,
		Href: href,
		Type: linkType,
	})
}

// AddEntry adds an entry to the feed.
func (f *Feed) AddEntry(entry Entry) {
	f.Entries = append(f.Entries, entry)
}

// Entry represents an OPDS entry (book or navigation item).
type Entry struct {
	ID        string    `xml:"id"`
	Title     string    `xml:"title"`
	Updated   time.Time `xml:"updated"`
	Published time.Time `xml:"published,omitempty"`
	Authors   []Author  `xml:"author,omitempty"`
	Summary   string    `xml:"summary,omitempty"`
	Content   *Content  `xml:"content,omitempty"`
	Links     []Link    `xml:"link"`
	// Dublin Core elements
	Language   string `xml:"dc:language,omitempty"`
	Publisher  string `xml:"dc:publisher,omitempty"`
	Identifier string `xml:"dc:identifier,omitempty"`
}

// NewEntry creates a new OPDS entry.
func NewEntry(id, title string) Entry {
	return Entry{
		ID:      id,
		Title:   title,
		Updated: time.Now().UTC(),
		Links:   []Link{},
	}
}

// AddLink adds a link to the entry.
func (e *Entry) AddLink(rel, href, linkType string) {
	e.Links = append(e.Links, Link{
		Rel:  rel,
		Href: href,
		Type: linkType,
	})
}

// AddAcquisitionLink adds a download link for a file.
func (e *Entry) AddAcquisitionLink(href, mimeType string) {
	e.Links = append(e.Links, Link{
		Rel:  RelAcquisition,
		Href: href,
		Type: mimeType,
	})
}

// AddImageLink adds a cover image link.
func (e *Entry) AddImageLink(href, mimeType string) {
	e.Links = append(e.Links, Link{
		Rel:  RelImage,
		Href: href,
		Type: mimeType,
	})
}

// AddThumbnailLink adds a thumbnail image link.
func (e *Entry) AddThumbnailLink(href, mimeType string) {
	e.Links = append(e.Links, Link{
		Rel:  RelThumbnail,
		Href: href,
		Type: mimeType,
	})
}

// Author represents an Atom author element.
type Author struct {
	Name string `xml:"name"`
	URI  string `xml:"uri,omitempty"`
}

// Link represents an Atom link element.
type Link struct {
	Rel         string `xml:"rel,attr,omitempty"`
	Href        string `xml:"href,attr"`
	Type        string `xml:"type,attr,omitempty"`
	Title       string `xml:"title,attr,omitempty"`
	FacetGroup  string `xml:"opds:facetGroup,attr,omitempty"`
	ActiveFacet bool   `xml:"opds:activeFacet,attr,omitempty"`
}

// Content represents entry content with type attribute.
type Content struct {
	Type  string `xml:"type,attr,omitempty"`
	Value string `xml:",chardata"`
}

// OpenSearchDescription represents an OpenSearch description document.
type OpenSearchDescription struct {
	XMLName        xml.Name        `xml:"OpenSearchDescription"`
	Xmlns          string          `xml:"xmlns,attr"`
	ShortName      string          `xml:"ShortName"`
	Description    string          `xml:"Description"`
	InputEncoding  string          `xml:"InputEncoding"`
	OutputEncoding string          `xml:"OutputEncoding"`
	URLs           []OpenSearchURL `xml:"Url"`
}

// OpenSearchURL represents an OpenSearch URL template.
type OpenSearchURL struct {
	Type     string `xml:"type,attr"`
	Template string `xml:"template,attr"`
}

// NewOpenSearchDescription creates a new OpenSearch description.
func NewOpenSearchDescription(shortName, description, searchTemplate string) *OpenSearchDescription {
	return &OpenSearchDescription{
		Xmlns:          OpenSearch,
		ShortName:      shortName,
		Description:    description,
		InputEncoding:  "UTF-8",
		OutputEncoding: "UTF-8",
		URLs: []OpenSearchURL{
			{
				Type:     MimeTypeAtom,
				Template: searchTemplate,
			},
		},
	}
}

// FileTypeMimeType returns the MIME type for a given file type.
func FileTypeMimeType(fileType string) string {
	switch fileType {
	case "epub":
		return MimeTypeEPUB
	case "cbz":
		return MimeTypeCBZ
	case "m4b":
		return MimeTypeM4B
	default:
		return "application/octet-stream"
	}
}

// CoverMimeType returns the MIME type for a given cover extension.
func CoverMimeType(ext string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return MimeTypeJPEG
	case ".png":
		return MimeTypePNG
	case ".webp":
		return MimeTypeWebP
	default:
		return MimeTypeJPEG
	}
}
