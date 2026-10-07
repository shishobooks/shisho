package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const goodMise = `[tools]
go = "1.26.8"
node = "24.21.0"
"npm:pnpm" = "10.33.0"
"github:gzuidhof/tygo" = "0.2.20"

[tasks.build]
run = "go = \"9.9.9\""
`

const goodDockerfile = `FROM --platform=$BUILDPLATFORM golang:1.26.8-alpine AS typegen
RUN go install github.com/gzuidhof/tygo@v0.2.20
FROM --platform=$BUILDPLATFORM node:24.21.0-alpine AS frontend-builder
FROM golang:1.26.8-alpine AS backend-builder
FROM alpine:3.23
`

const goodPackage = `{
  "packageManager": "pnpm@10.33.0",
  "dependencies": { "@types/node": "^24" }
}`

func TestParseMiseTools(t *testing.T) {
	t.Parallel()
	assert.Equal(t, map[string]string{
		"go":                   "1.26.8",
		"node":                 "24.21.0",
		"npm:pnpm":             "10.33.0",
		"github:gzuidhof/tygo": "0.2.20",
	}, ParseMiseTools([]byte(goodMise)))
}

func TestCheck(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		mise       string
		dockerfile string
		pkg        string
		want       []string
	}{
		{name: "all agree", mise: goodMise, dockerfile: goodDockerfile, pkg: goodPackage},
		{
			name:       "second golang image drifts",
			mise:       goodMise,
			dockerfile: strings.Replace(goodDockerfile, "FROM golang:1.26.8", "FROM golang:1.26.7", 1),
			pkg:        goodPackage,
			want:       []string{"golang:1.26.7"},
		},
		{
			name:       "node image drifts",
			mise:       goodMise,
			dockerfile: strings.Replace(goodDockerfile, "node:24.21.0", "node:24.20.0", 1),
			pkg:        goodPackage,
			want:       []string{"node:24.20.0"},
		},
		{
			name:       "tygo install drifts",
			mise:       goodMise,
			dockerfile: strings.Replace(goodDockerfile, "tygo@v0.2.20", "tygo@v0.2.19", 1),
			pkg:        goodPackage,
			want:       []string{"tygo@v0.2.19"},
		},
		{
			name:       "pnpm drifts",
			mise:       goodMise,
			dockerfile: goodDockerfile,
			pkg:        strings.Replace(goodPackage, "pnpm@10.33.0", "pnpm@10.32.0", 1),
			want:       []string{"pnpm@10.32.0"},
		},
		{
			name:       "@types/node major drifts",
			mise:       goodMise,
			dockerfile: goodDockerfile,
			pkg:        strings.Replace(goodPackage, `"^24"`, `"^22.1.0"`, 1),
			want:       []string{`@types/node "^22.1.0"`},
		},
		{
			name:       "@types/node minor may differ",
			mise:       goodMise,
			dockerfile: goodDockerfile,
			pkg:        strings.Replace(goodPackage, `"^24"`, `"^24.3.1"`, 1),
		},
		{
			name:       "missing pins fail loudly",
			mise:       "[tools]\ngo = \"1.26.8\"\n",
			dockerfile: "FROM alpine:3.23\n",
			pkg:        `{"packageManager": "npm@10.0.0"}`,
			want: []string{
				`no "node" pin`, `no "npm:pnpm" pin`, `no "github:gzuidhof/tygo" pin`,
				"no golang base image", "no node base image", "no `go install",
				"is not pnpm@<version>", "no @types/node",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Check([]byte(tc.mise), []byte(tc.dockerfile), []byte(tc.pkg))
			require.Len(t, got, len(tc.want), "problems: %q", got)
			for i, w := range tc.want {
				assert.Contains(t, got[i], w)
			}
		})
	}
}

// The repo's own pins must agree; this also proves the parsers still match
// the real files' formats.
func TestRepoPinsAgree(t *testing.T) {
	t.Parallel()
	read := func(name string) []byte {
		data, err := os.ReadFile("../../" + name)
		require.NoError(t, err)
		return data
	}
	assert.Empty(t, Check(read("mise.toml"), read("Dockerfile"), read("package.json")))
}
