package ereader

import (
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
)

// buildFilterQuery seeds a url.Values with the eReader's standard
// filter params (types, covers) plus any extra key/value pairs threaded
// through by the caller (e.g., "q" for search). Skips empty values so
// an unset filter doesn't show up as "?types=" in the link. Using
// url.Values ensures consistent encoding — values like a search query
// containing "&" or "#" are percent-encoded correctly and key order is
// stable across calls.
func buildFilterQuery(typesFilter, coversParam string, extraParams ...[2]string) url.Values {
	q := make(url.Values)
	if typesFilter != "" && typesFilter != "all" {
		q.Set("types", typesFilter)
	}
	if coversParam == "on" {
		q.Set("covers", "on")
	}
	for _, kv := range extraParams {
		if kv[1] == "" {
			continue
		}
		q.Set(kv[0], kv[1])
	}
	return q
}

const baseTemplate = `<!DOCTYPE html>
<html>
<head>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Shisho</title>
  <style>
    body { font-family: sans-serif; margin: 8px; }
    a { color: #000; text-decoration: underline; }
    a.item { display: block; text-decoration: none; }
    .item { padding: 12px 0; border-bottom: 1px solid #ccc; }
    .item-title { font-size: 1.1em; font-weight: bold; text-decoration: underline; }
    .item-meta { font-size: 0.9em; color: #666; }
    .nav { margin: 16px 0; }
    .nav-btn { display: inline-block; padding: 12px 16px; margin: 4px; border: 1px solid #000; text-decoration: none; }
    .filter { margin-bottom: 12px; }
    .filter-btn { display: inline-block; padding: 8px 12px; margin: 2px; border: 1px solid #ccc; text-decoration: none; }
  </style>
</head>
<body>
  %s
</body>
</html>`

// navBar generates a navigation bar with a home link.
// Uses button-style link for easier tapping on eReaders.
func navBar(homeURL string) string {
	if homeURL == "" {
		return ""
	}
	return fmt.Sprintf(`<div class="nav"><a href="%s" class="nav-btn">Home</a></div>`, html.EscapeString(homeURL))
}

// paginationWithParams generates pagination links with button styling,
// preserving filter params. extraParams contains additional query
// parameters to thread through each page link (e.g., a search query).
// Values are URL-encoded; keys must already be safe identifiers.
//
// currentPage is clamped into [1, totalPages] so a URL-edited ?page=
// overshoot renders a sensible "Page M of M" rather than "Page 9999 of 3"
// with misleading Prev/Next targets.
func paginationWithParams(currentPage, totalPages int, baseURL, typesFilter, coversParam string, extraParams ...[2]string) string {
	if totalPages <= 1 {
		return ""
	}
	if currentPage < 1 {
		currentPage = 1
	}
	if currentPage > totalPages {
		currentPage = totalPages
	}

	// Build query string with filter params
	buildURL := func(page int) string {
		q := buildFilterQuery(typesFilter, coversParam, extraParams...)
		q.Set("page", strconv.Itoa(page))
		return baseURL + "?" + q.Encode()
	}

	var parts []string
	if currentPage > 1 {
		parts = append(parts, fmt.Sprintf(`<a href="%s" class="nav-btn">← Prev</a>`, buildURL(currentPage-1)))
	} else {
		parts = append(parts, `<span class="nav-btn" style="color: #999;">← Prev</span>`)
	}

	parts = append(parts, fmt.Sprintf("Page %d of %d", currentPage, totalPages))

	if currentPage < totalPages {
		parts = append(parts, fmt.Sprintf(`<a href="%s" class="nav-btn">Next →</a>`, buildURL(currentPage+1)))
	} else {
		parts = append(parts, `<span class="nav-btn" style="color: #999;">Next →</span>`)
	}

	return fmt.Sprintf(`<div class="nav">%s</div>`, strings.Join(parts, " "))
}

// itemHTML generates an HTML item for lists.
// The entire row is wrapped in a link for easier tapping on eReaders.
func itemHTML(title, url, meta string) string {
	return fmt.Sprintf(`<a href="%s" class="item">
  <div class="item-title">%s</div>
  <div class="item-meta">%s</div>
</a>`, html.EscapeString(url), html.EscapeString(title), html.EscapeString(meta))
}

// searchForm generates a search form with basic styling for eReader compatibility.
// Uses inline styles and stacked layout for maximum browser support.
func searchForm(actionURL, query string) string {
	return fmt.Sprintf(`<form action="%s" method="get" style="margin: 16px 0;">
  <div style="margin-bottom: 8px;"><b>Search:</b></div>
  <div style="margin-bottom: 8px;">
    <input type="text" name="q" value="%s" style="display: block; width: 100%%; font-size: 18px; padding: 12px; border: 2px solid #000; box-sizing: border-box;">
  </div>
  <div>
    <input type="submit" value="Search" style="display: block; width: 100%%; font-size: 18px; padding: 12px; border: 2px solid #000; background: #eee;">
  </div>
</form>`, html.EscapeString(actionURL), html.EscapeString(query))
}

// filterBar generates the file type and cover filter UI with
// button-style links. extraParams preserves additional query params
// (e.g., "q" on the search page) so changing a filter doesn't drop the
// user's in-progress state.
func filterBar(baseURL, currentTypes, currentCovers string, extraParams ...[2]string) string {
	// Build type filter links
	typeLinks := []string{
		filterLink(baseURL, "types", "all", currentTypes, currentCovers, "All", extraParams...),
		filterLink(baseURL, "types", "epub", currentTypes, currentCovers, "EPUB", extraParams...),
		filterLink(baseURL, "types", "cbz", currentTypes, currentCovers, "CBZ", extraParams...),
		filterLink(baseURL, "types", "m4b", currentTypes, currentCovers, "M4B", extraParams...),
		filterLink(baseURL, "types", "pdf", currentTypes, currentCovers, "PDF", extraParams...),
	}

	// Build cover toggle links
	coverLinks := []string{
		filterLink(baseURL, "covers", "off", currentTypes, currentCovers, "Off", extraParams...),
		filterLink(baseURL, "covers", "on", currentTypes, currentCovers, "On", extraParams...),
	}

	return fmt.Sprintf(`<div class="filter">
  <div style="margin-bottom: 8px;"><b>Show:</b> %s</div>
  <div><b>Covers:</b> %s</div>
</div>`, strings.Join(typeLinks, " "), strings.Join(coverLinks, " "))
}

// filterLink generates a single filter link with button styling for easier tapping.
// The current selection is shown in bold without a link.
func filterLink(baseURL, param, value, currentTypes, currentCovers, label string, extraParams ...[2]string) string {
	// Determine query params
	types := currentTypes
	covers := currentCovers
	switch param {
	case "types":
		types = value
	case "covers":
		covers = value
	}

	q := buildFilterQuery(types, covers, extraParams...)
	linkURL := baseURL
	if encoded := q.Encode(); encoded != "" {
		linkURL += "?" + encoded
	}

	// Check if this is the current selection
	var isCurrent bool
	switch param {
	case "types":
		isCurrent = (value == currentTypes) || (value == "all" && currentTypes == "")
	case "covers":
		isCurrent = (value == currentCovers) || (value == "off" && currentCovers == "")
	}

	if isCurrent {
		return fmt.Sprintf(`<span class="filter-btn" style="font-weight: bold; border-color: #000;">%s</span>`, label)
	}
	return fmt.Sprintf(`<a href="%s" class="filter-btn">%s</a>`, html.EscapeString(linkURL), label)
}

// itemHTMLWithCover generates an HTML item with optional cover image.
// The entire row is wrapped in a link for easier tapping on eReaders.
func itemHTMLWithCover(title, url, meta, coverURL string, showCover bool) string {
	if showCover && coverURL != "" {
		return fmt.Sprintf(`<a href="%s" class="item">
  <img src="%s" alt="" style="max-width: 60px; max-height: 80px; float: left; margin-right: 8px;">
  <div class="item-title">%s</div>
  <div class="item-meta">%s</div>
  <div style="clear: both;"></div>
</a>`, html.EscapeString(url), html.EscapeString(coverURL), html.EscapeString(title), html.EscapeString(meta))
	}
	return itemHTML(title, url, meta)
}

// coverToggle generates a simple cover on/off toggle for the book detail page.
func coverToggle(baseURL, currentCovers string) string {
	offURL := baseURL
	onURL := baseURL + "?covers=on"

	var offLink, onLink string
	if currentCovers == "on" {
		offLink = fmt.Sprintf(`<a href="%s" class="filter-btn">Off</a>`, offURL)
		onLink = `<span class="filter-btn" style="font-weight: bold; border-color: #000;">On</span>`
	} else {
		offLink = `<span class="filter-btn" style="font-weight: bold; border-color: #000;">Off</span>`
		onLink = fmt.Sprintf(`<a href="%s" class="filter-btn">On</a>`, onURL)
	}

	return fmt.Sprintf(`<div class="filter"><b>Cover:</b> %s %s</div>`, offLink, onLink)
}

// RenderPage wraps content in the base template.
func RenderPage(content string) string {
	return fmt.Sprintf(baseTemplate, content)
}
