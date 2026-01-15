package main

import (
	"fmt"
	"os"

	"github.com/shishobooks/shisho/pkg/mp4"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <m4b-file>\n", os.Args[0])
		os.Exit(1)
	}

	path := os.Args[1]
	meta, err := mp4.ParseFull(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", path, err)
		os.Exit(1)
	}

	fmt.Printf("Title: %q\n", meta.Title)
	fmt.Printf("Subtitle: %q\n", meta.Subtitle)
	fmt.Printf("Album: %q\n", meta.Album)
	fmt.Printf("Series: %q\n", meta.Series)
	if meta.SeriesNumber != nil {
		fmt.Printf("SeriesNumber: %v\n", *meta.SeriesNumber)
	} else {
		fmt.Printf("SeriesNumber: nil\n")
	}
	fmt.Printf("Authors: %v\n", meta.Authors)
	fmt.Printf("Narrators: %v\n", meta.Narrators)
	fmt.Printf("Genre: %q\n", meta.Genre)
	fmt.Printf("Genres: %v\n", meta.Genres)
	fmt.Printf("Tags: %v\n", meta.Tags)
	fmt.Printf("Publisher: %q\n", meta.Publisher)
	fmt.Printf("Imprint: %q\n", meta.Imprint)
	fmt.Printf("URL: %q\n", meta.URL)
	fmt.Printf("Year: %q\n", meta.Year)
	fmt.Printf("ReleaseDate: %v\n", meta.ReleaseDate)
	fmt.Printf("Copyright: %q\n", meta.Copyright)
	fmt.Printf("Encoder: %q\n", meta.Encoder)
	fmt.Printf("Duration: %v\n", meta.Duration)
	fmt.Printf("Bitrate: %d bps\n", meta.Bitrate)
	fmt.Printf("MediaType: %d\n", meta.MediaType)
	fmt.Printf("Chapters: %d\n", len(meta.Chapters))
	fmt.Printf("Identifiers: %v\n", meta.Identifiers)
	fmt.Printf("HasCover: %v\n", len(meta.CoverData) > 0)
	if len(meta.CoverData) > 0 {
		fmt.Printf("CoverMimeType: %s\n", meta.CoverMimeType)
		fmt.Printf("CoverSize: %d bytes\n", len(meta.CoverData))
	}

	// Truncate long text fields for display
	if len(meta.Description) > 200 {
		fmt.Printf("Description: %q...\n", meta.Description[:200])
	} else {
		fmt.Printf("Description: %q\n", meta.Description)
	}
	if len(meta.Comment) > 200 {
		fmt.Printf("Comment: %q...\n", meta.Comment[:200])
	} else {
		fmt.Printf("Comment: %q\n", meta.Comment)
	}

	fmt.Printf("\nFreeform atoms (%d):\n", len(meta.Freeform))
	for k, v := range meta.Freeform {
		if len(v) > 100 {
			fmt.Printf("  %s: %s...\n", k, v[:100])
		} else {
			fmt.Printf("  %s: %s\n", k, v)
		}
	}

	fmt.Printf("\nUnknown atoms (%d):\n", len(meta.UnknownAtoms))
	for _, atom := range meta.UnknownAtoms {
		fmt.Printf("  %s (%d bytes)\n", string(atom.Type[:]), len(atom.Data))
	}

	if len(meta.Chapters) > 0 {
		fmt.Printf("\nChapters:\n")
		for i, ch := range meta.Chapters {
			fmt.Printf("  %d. %s [%v - %v]\n", i+1, ch.Title, ch.Start, ch.End)
		}
	}
}
