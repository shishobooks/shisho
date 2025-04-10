package main

import (
	"fmt"
	"os"

	"github.com/jessevdk/go-flags"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/epub"
)

func main() {
	log := logger.New()

	var opts struct {
		CoverOutput string `short:"o" long:"cover-output" description:"A path to output the cover image"`
	}

	args, err := flags.Parse(&opts)
	if err != nil {
		log.Err(err).Fatal("flags parse error")
	}

	if len(args) != 1 {
		fmt.Println("go run ./cmd/scripts/debug/parse-epub <path/to/file.epub>")
		os.Exit(1)
	}

	metadata, err := epub.Parse(args[0])
	if err != nil {
		log.Err(err).Fatal("epub parse error")
	}
	fmt.Printf("Title: %s\nAuthor(s): %v\nHas Cover Data: %v\nCover Mime Type: %s\n", metadata.Title, metadata.Authors, len(metadata.CoverData) > 0, metadata.CoverMimeType)
	if opts.CoverOutput != "" && metadata.CoverData != nil {
		f, err := os.Create(opts.CoverOutput)
		if err != nil {
			log.Err(err).Fatal("create file error")
		}
		_, err = f.Write(metadata.CoverData)
		if err != nil {
			log.Err(err).Fatal("file write error")
		}
	}
}
