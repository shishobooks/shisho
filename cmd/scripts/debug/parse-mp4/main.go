package main

import (
	"fmt"
	"os"

	"github.com/jessevdk/go-flags"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/mp4"
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
		fmt.Println("go run ./cmd/scripts/debug/parse-mp4 <path/to/file.mp4>")
		os.Exit(1)
	}

	metadata, err := mp4.Parse(args[0])
	if err != nil {
		log.Err(err).Fatal("mp4 parse error")
	}
	fmt.Println(metadata)
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
