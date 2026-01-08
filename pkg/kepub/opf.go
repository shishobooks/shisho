package kepub

import (
	"bytes"
	"io"
	"regexp"
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

// findCoverID finds the cover item ID from the metadata.
func findCoverID(content string) string {
	// Look for <meta name="cover" content="cover-id"/>
	metaPattern := regexp.MustCompile(`<meta[^>]+name\s*=\s*["']cover["'][^>]+content\s*=\s*["']([^"']+)["'][^>]*/?>`)
	matches := metaPattern.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}

	// Try alternative order: content before name
	metaPattern2 := regexp.MustCompile(`<meta[^>]+content\s*=\s*["']([^"']+)["'][^>]+name\s*=\s*["']cover["'][^>]*/?>`)
	matches = metaPattern2.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}

	// Look for item with properties="cover-image" (EPUB3)
	itemPattern := regexp.MustCompile(`<item[^>]+properties\s*=\s*["'][^"']*cover-image[^"']*["'][^>]+id\s*=\s*["']([^"']+)["']`)
	matches = itemPattern.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}

	// Try alternative order for EPUB3
	itemPattern2 := regexp.MustCompile(`<item[^>]+id\s*=\s*["']([^"']+)["'][^>]+properties\s*=\s*["'][^"']*cover-image[^"']*["']`)
	matches = itemPattern2.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// addCoverImageProperty adds the cover-image property to the manifest item with the given ID.
func addCoverImageProperty(content, coverID string) string {
	// Check if the item already has properties="cover-image"
	itemPattern := regexp.MustCompile(`<item[^>]+id\s*=\s*["']` + regexp.QuoteMeta(coverID) + `["'][^>]*>`)
	match := itemPattern.FindString(content)
	if match == "" {
		return content // Item not found
	}

	// Check if already has cover-image property
	if strings.Contains(match, "cover-image") {
		return content // Already has cover-image property
	}

	// Check if item has any properties attribute
	if strings.Contains(match, "properties=") {
		// Add cover-image to existing properties
		propertiesPattern := regexp.MustCompile(`(properties\s*=\s*["'])([^"']*)["']`)
		newMatch := propertiesPattern.ReplaceAllString(match, `${1}${2} cover-image"`)
		// Clean up double spaces
		newMatch = strings.ReplaceAll(newMatch, "  ", " ")
		return strings.Replace(content, match, newMatch, 1)
	}

	// Add properties attribute before the closing >
	newMatch := strings.TrimSuffix(match, "/>")
	newMatch = strings.TrimSuffix(newMatch, ">")
	if strings.HasSuffix(strings.TrimSpace(newMatch), "/") {
		newMatch = strings.TrimSuffix(strings.TrimSpace(newMatch), "/")
		newMatch += ` properties="cover-image"/>`
	} else {
		newMatch += ` properties="cover-image">`
	}

	return strings.Replace(content, match, newMatch, 1)
}

// TransformOPFBytes is a convenience function that transforms OPF from bytes.
func TransformOPFBytes(input []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := TransformOPF(bytes.NewReader(input), &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
