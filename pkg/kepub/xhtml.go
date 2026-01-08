package kepub

import (
	"bytes"
	"regexp"
	"strings"
)

// XHTMLProcessor handles conversion between XHTML and HTML5.
// Go's html package parses as HTML5, which mangles XHTML declarations.
// This processor preserves and restores XHTML-specific elements.
type XHTMLProcessor struct {
	xmlDeclaration string
}

// NewXHTMLProcessor creates a new XHTML processor.
func NewXHTMLProcessor() *XHTMLProcessor {
	return &XHTMLProcessor{}
}

// xmlDeclPattern matches XML declarations at the start of documents.
var xmlDeclPattern = regexp.MustCompile(`^<\?xml[^?]*\?>`)

// PreProcess extracts and removes the XML declaration from XHTML content.
// Call this before passing content to html.Parse.
func (p *XHTMLProcessor) PreProcess(content []byte) []byte {
	s := string(content)

	// Extract XML declaration
	if match := xmlDeclPattern.FindString(s); match != "" {
		p.xmlDeclaration = match
		s = strings.TrimPrefix(s, match)
	}

	return []byte(s)
}

// PostProcess converts HTML5 output back to XHTML format.
// Call this after html.Render to restore XHTML compliance.
func (p *XHTMLProcessor) PostProcess(content []byte) []byte {
	var buf bytes.Buffer

	// Restore XML declaration
	if p.xmlDeclaration != "" {
		buf.WriteString(p.xmlDeclaration)
		buf.WriteString("\n")
	}

	// Convert void elements to self-closing XML format
	result := convertToXHTML(content)
	buf.Write(result)

	return buf.Bytes()
}

// convertToXHTML converts HTML5 void elements to XHTML self-closing format.
func convertToXHTML(content []byte) []byte {
	s := string(content)

	// Remove any commented-out XML declarations that html.Parse might have created
	s = regexp.MustCompile(`<!--\?xml[^-]*-->`).ReplaceAllString(s, "")

	// HTML5 void elements that should be self-closing in XHTML
	// These elements in HTML5 have no closing tag, but in XHTML must be self-closed
	voidElements := []string{
		"area", "base", "br", "col", "embed", "hr", "img", "input",
		"link", "meta", "param", "source", "track", "wbr",
	}

	for _, elem := range voidElements {
		// Match <elem ...> and convert to <elem ... />
		// Handle both cases: with attributes and without
		pattern := regexp.MustCompile(`<(` + elem + `)(\s[^>]*)?>`)
		s = pattern.ReplaceAllStringFunc(s, func(match string) string {
			// Already self-closing
			if strings.HasSuffix(match, "/>") {
				return match
			}
			// Remove trailing > and add />
			return strings.TrimSuffix(match, ">") + "/>"
		})
	}

	// Convert empty paired elements to self-closing where appropriate
	// This is specifically for script and other elements that Kobo expects self-closing
	// <script ...></script> -> <script .../>
	scriptPattern := regexp.MustCompile(`<(script)(\s[^>]*)></script>`)
	s = scriptPattern.ReplaceAllString(s, "<$1$2/>")

	// Handle anchor elements that are self-closing in original XHTML
	// <a id="..." class="..."></a> -> <a id="..." class="..."/>
	anchorPattern := regexp.MustCompile(`<(a)(\s[^>]*)></a>`)
	s = anchorPattern.ReplaceAllString(s, "<$1$2/>")

	return []byte(s)
}
