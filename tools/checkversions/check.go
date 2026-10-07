package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// The mise.toml [tools] keys this check compares against the other pins.
const (
	miseGo   = "go"
	miseNode = "node"
	misePnpm = "npm:pnpm"
	miseTygo = "github:gzuidhof/tygo"
)

var (
	miseToolLine  = regexp.MustCompile(`^\s*"?([^"=\s]+)"?\s*=\s*"([^"]+)"`)
	golangImage   = regexp.MustCompile(`(?m)^FROM\s.*\bgolang:([0-9][^\s-]*)`)
	nodeImage     = regexp.MustCompile(`(?m)^FROM\s.*\bnode:([0-9][^\s-]*)`)
	tygoInstall   = regexp.MustCompile(`go install github\.com/gzuidhof/tygo@v?(\S+)`)
	leadingDigits = regexp.MustCompile(`[0-9]+`)
)

// ParseMiseTools returns the key/version pairs in mise.toml's [tools] table.
func ParseMiseTools(data []byte) map[string]string {
	tools := map[string]string{}
	inTools := false
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inTools = line == "[tools]"
			continue
		}
		if !inTools {
			continue
		}
		if m := miseToolLine.FindStringSubmatch(line); m != nil {
			tools[m[1]] = m[2]
		}
	}
	return tools
}

type packageJSON struct {
	PackageManager  string            `json:"packageManager"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

// Check compares the tool pins in the Dockerfile and package.json against
// mise.toml and returns one message per disagreement or missing pin. A pin
// that cannot be found is reported too, so a reformatted file fails loudly
// instead of passing unchecked.
func Check(mise, dockerfile, pkg []byte) []string {
	var problems []string
	tools := ParseMiseTools(mise)
	want := func(key string) string {
		v, ok := tools[key]
		if !ok {
			problems = append(problems, fmt.Sprintf("mise.toml: [tools] has no %q pin", key))
		}
		return v
	}
	goVersion, nodeVersion := want(miseGo), want(miseNode)
	pnpmVersion, tygoVersion := want(misePnpm), want(miseTygo)

	images := func(re *regexp.Regexp, image, key, version string) {
		matches := re.FindAllSubmatch(dockerfile, -1)
		if len(matches) == 0 {
			problems = append(problems, fmt.Sprintf("Dockerfile: no %s base image found", image))
		}
		for _, m := range matches {
			if got := string(m[1]); version != "" && got != version {
				problems = append(problems, fmt.Sprintf("Dockerfile: %s:%s does not match mise.toml %s = %q", image, got, key, version))
			}
		}
	}
	images(golangImage, "golang", miseGo, goVersion)
	images(nodeImage, "node", miseNode, nodeVersion)

	if m := tygoInstall.FindSubmatch(dockerfile); m == nil {
		problems = append(problems, "Dockerfile: no `go install github.com/gzuidhof/tygo@...` found")
	} else if got := string(m[1]); tygoVersion != "" && got != tygoVersion {
		problems = append(problems, fmt.Sprintf("Dockerfile: tygo@v%s does not match mise.toml %q = %q", got, miseTygo, tygoVersion))
	}

	var p packageJSON
	if err := json.Unmarshal(pkg, &p); err != nil {
		return append(problems, fmt.Sprintf("package.json: %v", err))
	}
	if got, ok := strings.CutPrefix(p.PackageManager, "pnpm@"); !ok {
		problems = append(problems, fmt.Sprintf("package.json: packageManager %q is not pnpm@<version>", p.PackageManager))
	} else if pnpmVersion != "" && got != pnpmVersion {
		problems = append(problems, fmt.Sprintf("package.json: packageManager pnpm@%s does not match mise.toml %q = %q", got, misePnpm, pnpmVersion))
	}

	typesNode, ok := p.Dependencies["@types/node"]
	if !ok {
		typesNode, ok = p.DevDependencies["@types/node"]
	}
	switch {
	case !ok:
		problems = append(problems, "package.json: no @types/node dependency")
	case nodeVersion != "" && major(typesNode) != major(nodeVersion):
		problems = append(problems, fmt.Sprintf("package.json: @types/node %q is not major %s from mise.toml node = %q", typesNode, major(nodeVersion), nodeVersion))
	}
	return problems
}

// major returns the first run of digits in a version or range ("^24.1" -> "24").
func major(v string) string {
	return leadingDigits.FindString(v)
}
