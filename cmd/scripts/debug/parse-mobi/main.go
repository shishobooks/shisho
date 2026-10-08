package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jessevdk/go-flags"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/mobi"
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
		fmt.Println("go run ./cmd/scripts/debug/parse-mobi <path/to/file.mobi|.azw3>")
		os.Exit(1)
	}

	metadata, err := mobi.Parse(args[0])
	if err != nil {
		log.Err(err).Fatal("mobi parse error")
	}
	out, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		log.Err(err).Fatal("marshal error")
	}
	fmt.Printf("%s\nHas Cover Data: %v\n", out, len(metadata.CoverData) > 0)
	if opts.CoverOutput != "" && metadata.CoverData != nil {
		if err := os.WriteFile(opts.CoverOutput, metadata.CoverData, 0600); err != nil {
			log.Err(err).Fatal("file write error")
		}
	}
}
