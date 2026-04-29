package main

import (
	"encoding/xml"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// History maps package import path → top-level test name → wallclock seconds.
type History map[string]map[string]float64

type junitTestSuites struct {
	XMLName    xml.Name         `xml:"testsuites"`
	TestSuites []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	XMLName   xml.Name        `xml:"testsuite"`
	Name      string          `xml:"name,attr"`
	TestCases []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Name string  `xml:"name,attr"`
	Time float64 `xml:"time,attr"`
}

// ReadHistory walks dir for *.xml files, parses them as JUnit XML, and returns
// a merged History. Subtests (Test/Subtest) are skipped — only top-level tests
// can be targeted with `go test -run`.
//
// Files are processed in lexical order; later files override earlier values
// for the same (pkg, test). A missing dir returns an empty History (no error).
func ReadHistory(dir string) (History, error) {
	h := History{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return h, nil
		}
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".xml") {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	sort.Strings(paths)
	for _, p := range paths {
		if err := mergeFile(h, p); err != nil {
			return nil, err
		}
	}
	return h, nil
}

func mergeFile(h History, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var ts junitTestSuites
	// Tolerate bare <testsuite> roots by wrapping if needed.
	if err := xml.Unmarshal(data, &ts); err != nil {
		var single junitTestSuite
		if err2 := xml.Unmarshal(data, &single); err2 == nil {
			ts.TestSuites = []junitTestSuite{single}
		} else {
			return err
		}
	}
	for _, suite := range ts.TestSuites {
		if suite.Name == "" {
			continue
		}
		pkg := h[suite.Name]
		if pkg == nil {
			pkg = map[string]float64{}
			h[suite.Name] = pkg
		}
		for _, tc := range suite.TestCases {
			if strings.Contains(tc.Name, "/") {
				continue // subtest, skip
			}
			pkg[tc.Name] = tc.Time
		}
	}
	return nil
}
