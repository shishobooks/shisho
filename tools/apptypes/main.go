// Command apptypes writes the API types as Swift Codable and Kotlin
// serializable types for the Shisho app, from the same Go packages tygo
// reads for the web app (ADR 0021). The decoders it writes are lenient, as
// ADR 0020 requires: unknown keys are ignored, every enum keeps an unknown
// value in an unknown case, and a field the server leaves out or sends as
// null takes its zero value, or nil for a Go pointer.
//
// `mise tygo` runs it, writing into each app's gitignored generated folder.
// With -check it writes nothing and fails when those folders are stale.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
)

func main() {
	config := flag.String("config", "tygo.yaml", "tygo config naming the packages")
	swiftDir := flag.String("swift", "mobile/ios/ShishoAPI/Sources/ShishoAPI/Generated", "folder for the Swift types")
	kotlinDir := flag.String("kotlin", "mobile/android/api/src/main/kotlin/generated", "folder for the Kotlin types")
	check := flag.Bool("check", false, "fail if the folders don't hold what would be written, instead of writing")
	flag.Parse()

	stale, err := run(*config, *swiftDir, *kotlinDir, *check)
	if err != nil {
		fmt.Fprintln(os.Stderr, "apptypes:", err)
		os.Exit(2)
	}
	if len(stale) > 0 {
		for _, path := range stale {
			fmt.Println(path)
		}
		fmt.Fprintf(os.Stderr, "apptypes: %d generated Swift or Kotlin file(s) are out of date with the Go API types. Run `mise tygo`.\n", len(stale))
		os.Exit(1)
	}
}

// run generates every file and writes it, or with check lists the stale
// ones.
func run(config, swiftDir, kotlinDir string, check bool) ([]string, error) {
	model, err := load(config)
	if err != nil {
		return nil, err
	}
	if err := checkNames(model); err != nil {
		return nil, err
	}
	var stale []string
	for dir, files := range map[string]map[string]string{
		swiftDir:  generateSwift(model),
		kotlinDir: generateKotlin(model),
	} {
		paths, err := sync(dir, files, check)
		if err != nil {
			return nil, err
		}
		stale = append(stale, paths...)
	}
	sort.Strings(stale)
	return stale, nil
}

// checkNames fails when two types of one package would share a name.
func checkNames(m *Model) error {
	for _, pkg := range m.Packages {
		seen := map[string]string{}
		var names []string
		for _, s := range pkg.Structs {
			names = append(names, s.Name)
		}
		for _, e := range pkg.Enums {
			names = append(names, e.Name)
		}
		for _, a := range pkg.Aliases {
			names = append(names, a.Name)
		}
		for _, name := range names {
			if _, ok := seen[name]; ok {
				return errors.Errorf("package %s declares two types named %s; rename one", pkg.Name, name)
			}
			seen[name] = pkg.Name
		}
	}
	return nil
}

// sync makes dir hold exactly files. With check it changes nothing and
// returns the paths that differ, are missing, or shouldn't be there. A dir
// that was never generated, as in a fresh checkout, has nothing stale.
func sync(dir string, files map[string]string, check bool) ([]string, error) {
	existing, err := os.ReadDir(dir)
	if os.IsNotExist(err) && check {
		return nil, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, errors.WithStack(err)
	}
	var stale []string
	for _, entry := range existing {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if _, ok := files[entry.Name()]; ok {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if check {
			stale = append(stale, path)
			continue
		}
		if err := os.Remove(path); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	if !check {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		current, err := os.ReadFile(path)
		if err == nil && bytes.Equal(current, []byte(content)) {
			continue
		}
		if check {
			stale = append(stale, path)
			continue
		}
		// Generated source, readable like the rest of the checkout.
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // Source files, not secrets.
			return nil, errors.WithStack(err)
		}
	}
	return stale, nil
}
