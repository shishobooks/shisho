package kepub

import (
	"bytes"
	"io"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// TransformOPF transforms an OPF file for KePub compatibility.
// It ensures the cover image has the cover-image property set.
func TransformOPF(r io.Reader, w io.Writer) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	result := transformOPFContent(data)
	_, err = w.Write(result)
	return err
}

// transformOPFContent transforms OPF content for KePub compatibility.
func transformOPFContent(data []byte) []byte {
	content := string(data)

	// Find cover item ID from meta tag
	coverID := findCoverID(content)
	if coverID == "" {
		return data // No cover defined, return unchanged
	}

	// Add cover-image property to the manifest item
	content = addCoverImageProperty(content, coverID)

	return []byte(content)
}

// Patterns for finding the cover in an OPF, compiled once.
var (
	// <meta name="cover" content="cover-id"/>, in either attribute order.
	coverMetaPattern             = regexp.MustCompile(`<meta[^>]+name\s*=\s*["']cover["'][^>]+content\s*=\s*["']([^"']+)["'][^>]*/?>`)
	coverMetaContentFirstPattern = regexp.MustCompile(`<meta[^>]+content\s*=\s*["']([^"']+)["'][^>]+name\s*=\s*["']cover["'][^>]*/?>`)
	// An item with properties="cover-image" (EPUB 3), in either attribute
	// order.
	coverItemPattern        = regexp.MustCompile(`<item[^>]+properties\s*=\s*["'][^"']*cover-image[^"']*["'][^>]+id\s*=\s*["']([^"']+)["']`)
	coverItemIDFirstPattern = regexp.MustCompile(`<item[^>]+id\s*=\s*["']([^"']+)["'][^>]+properties\s*=\s*["'][^"']*cover-image[^"']*["']`)
	propertiesPattern       = regexp.MustCompile(`(\sproperties\s*=\s*)(?:"([^"]*)"|'([^']*)')`)
)

// findCoverID finds the cover item ID from the metadata.
func findCoverID(content string) string {
	// Look for <meta name="cover" content="cover-id"/>
	matches := coverMetaPattern.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}

	// Try alternative order: content before name
	matches = coverMetaContentFirstPattern.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}

	// Look for item with properties="cover-image" (EPUB3)
	matches = coverItemPattern.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}

	// Try alternative order for EPUB3
	matches = coverItemIDFirstPattern.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// addCoverImageProperty adds the cover-image property to the manifest item with the given ID.
func addCoverImageProperty(content, coverID string) string {
	itemPattern := regexp.MustCompile(`<item[^>]+id\s*=\s*["']` + regexp.QuoteMeta(coverID) + `["'][^>]*>`)
	match := itemPattern.FindString(content)
	if match == "" {
		return content // Item not found
	}
	return strings.Replace(content, match, addItemProperty(match, "cover-image"), 1)
}

var (
	packageVersionPattern = regexp.MustCompile(`<package\b[^>]*\bversion\s*=\s*["']\s*(\d+)`)
	manifestItemPattern   = regexp.MustCompile(`<item\b[^>]*>`)
)

// transformKepubOPF transforms the OPF at opfPath for a KePub: the cover
// gets the cover-image property, and the kobo.js at the root of the zip
// that every content document loads is declared in the manifest. In EPUB 3
// the documents in contentFiles are also marked scripted, as a document
// that runs a script must be.
func transformKepubOPF(data []byte, opfPath string, contentFiles map[string]bool) []byte {
	content := string(transformOPFContent(data))
	epub3 := false
	if m := packageVersionPattern.FindStringSubmatch(content); m != nil {
		major, err := strconv.Atoi(m[1])
		epub3 = err == nil && major >= 3
	}

	declared := false
	content = manifestItemPattern.ReplaceAllStringFunc(content, func(tag string) string {
		entry := manifestEntryPath(opfPath, extractAttribute(tag, "href"))
		if entry == koboJSPath {
			declared = true
		}
		if epub3 && contentFiles[entry] {
			return addItemProperty(tag, "scripted")
		}
		return tag
	})
	if declared {
		return []byte(content)
	}

	id := "kobo-js"
	for i := 2; strings.Contains(content, `id="`+id+`"`) || strings.Contains(content, `id='`+id+`'`); i++ {
		id = "kobo-js-" + strconv.Itoa(i)
	}
	item := `<item id="` + id + `" href="` + computeRelativeScriptPath(opfPath) + `" media-type="application/javascript"/>`
	if i := strings.LastIndex(content, "</manifest>"); i >= 0 {
		content = content[:i] + item + content[i:]
	}
	return []byte(content)
}

// manifestEntryPath resolves a manifest href to its zip entry name. Hrefs
// are relative to the OPF and may be percent-encoded.
func manifestEntryPath(opfPath, href string) string {
	if unescaped, err := url.PathUnescape(href); err == nil {
		href = unescaped
	}
	return path.Join(path.Dir(opfPath), href)
}

// addItemProperty adds prop to a manifest item tag's properties, unless it
// is there already.
func addItemProperty(tag, prop string) string {
	if loc := propertiesPattern.FindStringSubmatchIndex(tag); loc != nil {
		// The value is group 2 when double-quoted and group 3 when single-
		// quoted; the closing quote follows it.
		start, end := loc[4], loc[5]
		if start < 0 {
			start, end = loc[6], loc[7]
		}
		// Check the properties tokens, not the whole tag: an id or href can
		// contain the property's name too.
		tokens := strings.Fields(tag[start:end])
		if slices.Contains(tokens, prop) {
			return tag
		}
		return tag[:start] + strings.Join(append(tokens, prop), " ") + tag[end:]
	}

	// Add properties attribute before the closing > or />
	if trimmed, ok := strings.CutSuffix(tag, "/>"); ok {
		return strings.TrimRight(trimmed, " \t\r\n") + ` properties="` + prop + `"/>`
	}
	return strings.TrimSuffix(tag, ">") + ` properties="` + prop + `">`
}

// removeItemProperty removes prop from a manifest item tag's properties,
// and the properties attribute when nothing is left in it.
func removeItemProperty(tag, prop string) string {
	loc := propertiesPattern.FindStringSubmatchIndex(tag)
	if loc == nil {
		return tag
	}
	// The value is group 2 when double-quoted and group 3 when single-
	// quoted.
	start, end := loc[4], loc[5]
	if start < 0 {
		start, end = loc[6], loc[7]
	}
	tokens := strings.Fields(tag[start:end])
	kept := slices.DeleteFunc(slices.Clone(tokens), func(t string) bool { return t == prop })
	if len(kept) == len(tokens) {
		return tag
	}
	if len(kept) == 0 {
		return tag[:loc[0]] + tag[loc[1]:]
	}
	return tag[:start] + strings.Join(kept, " ") + tag[end:]
}

// TransformOPFBytes is a convenience function that transforms OPF from bytes.
func TransformOPFBytes(input []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := TransformOPF(bytes.NewReader(input), &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
