package main

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/pkg/errors"
)

type opdsFeed struct {
	Links   []opdsLink  `xml:"link"`
	Entries []opdsEntry `xml:"entry"`
}

type opdsEntry struct {
	Title string     `xml:"title"`
	Links []opdsLink `xml:"link"`
}

type opdsLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
	Type string `xml:"type,attr"`
}

// findAcquisition walks an OPDS catalog, as a person browsing would, for the
// first book whose title contains title, and returns its download URL.
func findAcquisition(ctx context.Context, server, username, password, title string) (string, error) {
	seen := map[string]bool{}
	queue := []string{strings.TrimRight(server, "/") + "/opds/v1/catalog"}
	want := strings.ToLower(title)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			continue
		}
		seen[current] = true

		feed, err := fetchFeed(ctx, current, username, password)
		if err != nil {
			return "", err
		}
		base, err := url.Parse(current)
		if err != nil {
			return "", errors.WithStack(err)
		}
		resolve := func(href string) string {
			ref, err := url.Parse(href)
			if err != nil {
				return href
			}
			return base.ResolveReference(ref).String()
		}
		for _, entry := range feed.Entries {
			for _, link := range entry.Links {
				if strings.HasPrefix(link.Rel, "http://opds-spec.org/acquisition") {
					if strings.Contains(strings.ToLower(entry.Title), want) {
						return resolve(link.Href), nil
					}
					continue
				}
				if strings.Contains(link.Type, "atom+xml") {
					queue = append(queue, resolve(link.Href))
				}
			}
		}
		for _, link := range feed.Links {
			if link.Rel == "next" {
				queue = append(queue, resolve(link.Href))
			}
		}
	}
	return "", errors.Errorf("no book titled %q in the catalog", title)
}

func fetchFeed(ctx context.Context, feedURL, username, password string) (*opdsFeed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	req.SetBasicAuth(username, password)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("%s: %s", feedURL, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	feed := &opdsFeed{}
	if err := xml.Unmarshal(body, feed); err != nil {
		return nil, errors.Wrapf(err, "parse %s", feedURL)
	}
	return feed, nil
}
