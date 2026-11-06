// Command koreaderharness drives a real, headless KOReader in Docker, so
// KOReader sync can be checked without a device. Every command prints JSON.
// See AGENTS.md beside this file.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

const usage = `Drives a real, headless KOReader against a running Shisho. Every command prints JSON.

  mise koreader <command> [flags]

  login      --server URL --username U --password P --file BOOK   sign in to progress sync
  register   --server URL --username U --password P --file BOOK   tap Register
  download   --server URL --username U --password P (--title T | --url URL) [--as NAME]
             download over OPDS, named after its URL unless --as names it
  open       --file BOOK                                  report hash, pages, position, percentage
  goto       --file BOOK (--page N | --percent N | --xpointer X)
  push       --file BOOK                                  push through KOReader's sync plugin
  pull       --file BOOK [--mode silent|prompt|interactive] [--confirm]
             interactive is the menu's pull, which always jumps. silent and prompt are the
             pull KOReader does on opening and waking; prompt shows its jump dialog, and
             --confirm accepts it.
  queue-push --file BOOK                                  push while offline, which queues it
  drain      --file BOOK                                  send what's queued
  screenshot --file BOOK --out PNG
  digest     --file PATH                                  KOReader's document hash of any file
  xpointers  --file BOOK                                  every word start of a reflowable book, as
             KOReader writes its XPointer, with its text: the expected values for translation tests

  Any command that opens a book takes --out PNG to screenshot the result, and
  --view page|scroll for KOReader's page or continuous view of a reflowable book.
  --file is a book in the harness's books directory, or a path to copy there.
  --server is what this machine reaches Shisho at, such as the API URL mise start prints.
  KOReader's home, sign-in, and positions live in tmp/koreader; delete it to start over.
`

func main() {
	failed, err := run(os.Args[1:])
	if err != nil && !failed {
		// A failure before KOReader ran prints JSON too.
		encoded, jsonErr := json.Marshal(map[string]any{"ok": false, "error": err.Error()})
		if jsonErr != nil {
			fmt.Fprintln(os.Stderr, err)
		} else {
			fmt.Println(string(encoded))
		}
	}
	if err != nil {
		os.Exit(1)
	}
}

// run runs one command. printed is true when it already printed its result.
func run(argv []string) (printed bool, err error) {
	if len(argv) == 0 || argv[0] == "help" || argv[0] == "--help" || argv[0] == "-h" {
		fmt.Print(usage)
		return false, nil
	}
	command := argv[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	server := flags.String("server", "", "Shisho's URL")
	username := flags.String("username", "", "Shisho username")
	password := flags.String("password", "", "the Device's credential")
	file := flags.String("file", "", "book to open")
	title := flags.String("title", "", "title to find in the OPDS catalog")
	downloadURL := flags.String("url", "", "OPDS download URL")
	page := flags.Int("page", 0, "1-based page")
	percent := flags.String("percent", "", "percentage through the book")
	xpointer := flags.String("xpointer", "", "XPointer")
	mode := flags.String("mode", "", "silent, prompt, or interactive")
	view := flags.String("view", "", "page or scroll, KOReader's page and continuous views")
	as := flags.String("as", "", "name to save a download under")
	confirm := flags.Bool("confirm", false, "accept KOReader's jump dialog")
	out := flags.String("out", "", "PNG to screenshot to")
	dir := flags.String("dir", filepath.Join("tmp", "koreader"), "harness state directory")
	flags.SetOutput(io.Discard)
	if err := flags.Parse(argv[1:]); err != nil {
		return false, err
	}
	percentValue, err := percentArg(*percent)
	if err != nil {
		return false, err
	}

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		return false, errors.WithStack(err)
	}
	h := &Harness{Dir: absDir, ReleaseDir: filepath.Join(absDir, "release")}
	ctx := context.Background()
	args := Args{
		Command: command, File: *file, Server: *server, Username: *username, Password: *password,
		Page: *page, Percent: percentValue, XPointer: *xpointer, Mode: *mode, View: *view, Confirm: *confirm, Screenshot: *out,
	}
	if command == "download" {
		target := *downloadURL
		if target == "" {
			target, err = findAcquisition(ctx, *server, *username, *password, *title)
			if err != nil {
				return false, err
			}
		}
		args.URL = target
		args.Output = downloadName(target)
		if *as != "" {
			args.Output = *as
		}
		args.Server = ""
	}
	result, err := h.Run(ctx, args)
	if err != nil {
		return false, err
	}
	fmt.Println(string(result.Raw))
	if !result.OK {
		return true, errors.Errorf("koreader: %s", result.Error)
	}
	return true, nil
}

// downloadName names a download by its URL, which already ends in the File's
// id and extension, so two Files don't overwrite each other.
func downloadName(target string) string {
	name := path.Base(strings.SplitN(target, "?", 2)[0])
	if name == "." || name == "/" {
		return "download"
	}
	return name
}
