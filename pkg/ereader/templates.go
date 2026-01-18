package ereader

import (
	"fmt"
	"html"
	"strconv"
	"strings"
)

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

// paginationWithParams generates pagination links with button styling, preserving filter params.
func paginationWithParams(currentPage, totalPages int, baseURL, typesFilter, coversParam string) string {
	if totalPages <= 1 {
		return ""
	}

	// Build query string with filter params
	buildURL := func(page int) string {
		url := baseURL + "?page=" + strconv.Itoa(page)
		if typesFilter != "" && typesFilter != "all" {
			url += "&types=" + typesFilter
		}
		if coversParam == "on" {
			url += "&covers=on"
		}
		return url
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

// filterBar generates the file type and cover filter UI with button-style links.
func filterBar(baseURL, currentTypes, currentCovers string) string {
	// Build type filter links
	typeLinks := []string{
		filterLink(baseURL, "types", "all", currentTypes, currentCovers, "All"),
		filterLink(baseURL, "types", "epub", currentTypes, currentCovers, "EPUB"),
		filterLink(baseURL, "types", "cbz", currentTypes, currentCovers, "CBZ"),
		filterLink(baseURL, "types", "m4b", currentTypes, currentCovers, "M4B"),
	}

	// Build cover toggle links
	coverLinks := []string{
		filterLink(baseURL, "covers", "off", currentTypes, currentCovers, "Off"),
		filterLink(baseURL, "covers", "on", currentTypes, currentCovers, "On"),
	}

	return fmt.Sprintf(`<div class="filter">
  <div style="margin-bottom: 8px;"><b>Show:</b> %s</div>
  <div><b>Covers:</b> %s</div>
</div>`, strings.Join(typeLinks, " "), strings.Join(coverLinks, " "))
}

// filterLink generates a single filter link with button styling for easier tapping.
// The current selection is shown in bold without a link.
func filterLink(baseURL, param, value, currentTypes, currentCovers, label string) string {
	// Determine query params
	types := currentTypes
	covers := currentCovers
	switch param {
	case "types":
		types = value
	case "covers":
		covers = value
	}

	// Build URL with query params
	url := baseURL
	sep := "?"
	if types != "" && types != "all" {
		url += sep + "types=" + types
		sep = "&"
	}
	if covers == "on" {
		url += sep + "covers=on"
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
	return fmt.Sprintf(`<a href="%s" class="filter-btn">%s</a>`, html.EscapeString(url), label)
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
