package epub

import (
	"encoding/xml"
	"io"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/mediafile"
)

// NavHTML represents the EPUB 3 navigation document structure.
type NavHTML struct {
	XMLName xml.Name `xml:"html"`
	Body    struct {
		Nav []NavElement `xml:"nav"`
	} `xml:"body"`
}

// NavElement represents a nav element in the navigation document.
type NavElement struct {
	Type string `xml:"type,attr"`
	OL   *NavOL `xml:"ol"`
}

// NavOL represents an ordered list in the navigation.
type NavOL struct {
	Items []NavLI `xml:"li"`
}

// NavLI represents a list item in the navigation.
type NavLI struct {
	A        *NavLink `xml:"a"`
	Span     *NavSpan `xml:"span"`
	Children *NavOL   `xml:"ol"`
}

// NavLink represents an anchor element.
type NavLink struct {
	Href string `xml:"href,attr"`
	Text string `xml:",chardata"`
}

// NavSpan represents a span element (heading without link).
type NavSpan struct {
	Text string `xml:",chardata"`
}

// parseNavDocument parses an EPUB 3 navigation document and returns chapters.
func parseNavDocument(r io.Reader) ([]mediafile.ParsedChapter, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var nav NavHTML
	if err := xml.Unmarshal(data, &nav); err != nil {
		return nil, errors.WithStack(err)
	}

	// Find the toc nav element
	for _, n := range nav.Body.Nav {
		if n.Type == "toc" && n.OL != nil {
			return parseNavOL(n.OL), nil
		}
	}

	return nil, nil
}

// parseNavOL recursively parses an ordered list into chapters.
func parseNavOL(ol *NavOL) []mediafile.ParsedChapter {
	if ol == nil {
		return nil
	}

	chapters := make([]mediafile.ParsedChapter, 0, len(ol.Items))
	for _, li := range ol.Items {
		ch := mediafile.ParsedChapter{}

		// Get title and href from anchor or span
		if li.A != nil {
			ch.Title = strings.TrimSpace(li.A.Text)
			if li.A.Href != "" {
				href := li.A.Href
				ch.Href = &href
			}
		} else if li.Span != nil {
			ch.Title = strings.TrimSpace(li.Span.Text)
		}

		// Skip items without a title
		if ch.Title == "" {
			continue
		}

		// Parse nested children
		if li.Children != nil {
			ch.Children = parseNavOL(li.Children)
		}

		chapters = append(chapters, ch)
	}

	return chapters
}

// NCX represents the EPUB 2 NCX structure.
type NCX struct {
	XMLName xml.Name `xml:"ncx"`
	NavMap  struct {
		NavPoints []NCXNavPoint `xml:"navPoint"`
	} `xml:"navMap"`
}

// NCXNavPoint represents a navigation point in NCX.
type NCXNavPoint struct {
	ID       string `xml:"id,attr"`
	NavLabel struct {
		Text string `xml:"text"`
	} `xml:"navLabel"`
	Content struct {
		Src string `xml:"src,attr"`
	} `xml:"content"`
	Children []NCXNavPoint `xml:"navPoint"`
}

// parseNCX parses an EPUB 2 NCX file and returns chapters.
func parseNCX(r io.Reader) ([]mediafile.ParsedChapter, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var ncx NCX
	if err := xml.Unmarshal(data, &ncx); err != nil {
		return nil, errors.WithStack(err)
	}

	return parseNCXNavPoints(ncx.NavMap.NavPoints), nil
}

// parseNCXNavPoints recursively parses NCX navigation points.
func parseNCXNavPoints(navPoints []NCXNavPoint) []mediafile.ParsedChapter {
	chapters := make([]mediafile.ParsedChapter, 0, len(navPoints))
	for _, np := range navPoints {
		title := strings.TrimSpace(np.NavLabel.Text)
		if title == "" {
			continue
		}

		ch := mediafile.ParsedChapter{
			Title: title,
		}

		if np.Content.Src != "" {
			src := np.Content.Src
			ch.Href = &src
		}

		if len(np.Children) > 0 {
			ch.Children = parseNCXNavPoints(np.Children)
		}

		chapters = append(chapters, ch)
	}
	return chapters
}

// findNavDocumentHref finds the navigation document href from an OPF package.
// Returns empty string if not found.
func findNavDocumentHref(pkg *Package, basePath string) string {
	for _, item := range pkg.Manifest.Item {
		if strings.Contains(item.Properties, "nav") {
			return basePath + item.Href
		}
	}
	return ""
}

// findNCXHref finds the NCX file href from an OPF package.
// Returns empty string if not found.
func findNCXHref(pkg *Package, basePath string) string {
	ncxID := pkg.Spine.Toc
	if ncxID == "" {
		return ""
	}
	for _, item := range pkg.Manifest.Item {
		if item.ID == ncxID {
			return basePath + item.Href
		}
	}
	return ""
}
