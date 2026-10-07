// Package goconsts reads string constants out of Go source so a test can
// check that every constant of a family is handled, without a hand-kept list
// that goes stale the same way the code under test would.
package goconsts

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// StringsWithPrefix parses the non-test Go files in dir and returns the
// values of every package-level constant whose name starts with prefix,
// sorted. Each matching constant must be a plain string literal; anything
// else (iota, expressions) fails the test so it is never skipped silently.
func StringsWithPrefix(t testing.TB, dir, prefix string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	fset := token.NewFileSet()
	var values []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, ident := range vs.Names {
					if !strings.HasPrefix(ident.Name, prefix) {
						continue
					}
					require.Less(t, i, len(vs.Values), "constant %s has no value of its own", ident.Name)
					lit, ok := vs.Values[i].(*ast.BasicLit)
					require.True(t, ok && lit.Kind == token.STRING, "constant %s must be a string literal", ident.Name)
					value, err := strconv.Unquote(lit.Value)
					require.NoError(t, err)
					values = append(values, value)
				}
			}
		}
	}
	require.NotEmpty(t, values, "no constants named %s* in %s", prefix, dir)
	sort.Strings(values)
	return values
}
