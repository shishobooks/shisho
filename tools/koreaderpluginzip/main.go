// Command koreaderpluginzip writes the KOReader plugin's zip as a release
// attaches it: stamped with the release's version and the plugin's hash, but
// no server or short code, for installing by hand.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	pluginfiles "github.com/shishobooks/shisho/koreader-plugin"
	"github.com/shishobooks/shisho/pkg/koreaderplugin"
)

func main() {
	version := flag.String("version", "", "the release's version, such as v0.8.0")
	out := flag.String("out", "", "where to write the zip")
	flag.Parse()
	if *version == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: koreaderpluginzip -version vX.Y.Z -out FILE")
		os.Exit(2)
	}
	if err := write(*version, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func write(version, out string) error {
	bundle, err := koreaderplugin.NewBundle(pluginfiles.FS, version)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	//nolint:gosec // A release asset, which anyone may read.
	return os.WriteFile(out, bundle.Zip(), 0o644)
}
