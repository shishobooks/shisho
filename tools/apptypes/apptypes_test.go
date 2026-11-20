package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadFixture(t *testing.T) *Model {
	t.Helper()
	m, err := load(filepath.Join("testdata", "tygo.yaml"))
	require.NoError(t, err)
	require.NoError(t, checkNames(m))
	return m
}

func findStruct(t *testing.T, m *Model, pkg, name string) *Struct {
	t.Helper()
	s := m.structs[pkg+"."+name]
	require.NotNil(t, s, "%s.%s", pkg, name)
	return s
}

func fieldTypes(s *Struct) map[string]TypeRef {
	out := map[string]TypeRef{}
	for _, f := range s.Fields {
		out[f.JSONName] = f.Type
	}
	return out
}

func TestLoad_MapsGoTypesAsEncodingJSONSendsThem(t *testing.T) {
	t.Parallel()
	m := loadFixture(t)
	widget := findStruct(t, m, "fixture", "Widget")
	assert.Equal(t, "Widget is documented.", widget.Doc)

	var names []string
	for _, f := range widget.Fields {
		names = append(names, f.JSONName)
	}
	// Embedded fields come first, as encoding/json orders them; the
	// shallower shadow wins; "-" fields and unexported ones are left out.
	assert.Equal(t, []string{"id", "shadow", "name", "maybe", "kind", "kinds", "color", "at", "size", "small", "ratio",
		"tags", "raw", "any", "shared", "mine", "parent", "children", "by_id", "NoTag"}, names)

	types := fieldTypes(widget)
	assert.Equal(t, TypeRef{Kind: KindInt}, types["shadow"])
	assert.Equal(t, TypeRef{Kind: KindString, Optional: true}, types["maybe"])
	assert.Equal(t, TypeRef{Kind: KindEnum, Name: "Kind", Pkg: "fixture"}, types["kind"])
	assert.Equal(t, TypeRef{Kind: KindArray, Elem: &TypeRef{Kind: KindEnum, Name: "Kind", Pkg: "fixture"}}, types["kinds"])
	assert.Equal(t, TypeRef{Kind: KindEnum, Name: "Color", Pkg: "other", Optional: true}, types["color"])
	assert.Equal(t, TypeRef{Kind: KindString}, types["at"])
	assert.Equal(t, TypeRef{Kind: KindInt}, types["size"])
	assert.Equal(t, TypeRef{Kind: KindInt32}, types["small"])
	assert.Equal(t, TypeRef{Kind: KindDouble}, types["ratio"])
	assert.Equal(t, TypeRef{Kind: KindMap, Elem: &TypeRef{Kind: KindInt}}, types["tags"])
	assert.Equal(t, TypeRef{Kind: KindJSON}, types["raw"])
	assert.Equal(t, TypeRef{Kind: KindJSON}, types["any"])
	assert.Equal(t, TypeRef{Kind: KindNamed, Name: "Shared", Pkg: "other"}, types["shared"])
	assert.Equal(t, TypeRef{Kind: KindNamed, Name: "Shared", Pkg: "fixture"}, types["mine"])
	// A Widget can't hold a Widget by value in Swift, so its parent is boxed;
	// arrays and maps already hold their elements indirectly.
	assert.Equal(t, TypeRef{Kind: KindNamed, Name: "Widget", Pkg: "fixture", Optional: true, Indirect: true}, types["parent"])
	assert.Equal(t, TypeRef{Kind: KindArray, Elem: &TypeRef{Kind: KindNamed, Name: "Widget", Pkg: "fixture"}}, types["children"])
	assert.Equal(t, TypeRef{Kind: KindMap, Elem: &TypeRef{Kind: KindNamed, Name: "Widget", Pkg: "fixture"}}, types["by_id"])

	var fixture *Package
	for _, pkg := range m.Packages {
		if pkg.Name == "fixture" {
			fixture = pkg
		}
	}
	require.NotNil(t, fixture)
	require.Len(t, fixture.Enums, 1)
	assert.Equal(t, []string{"book", "default"}, fixture.Enums[0].Values)
	require.Len(t, fixture.Aliases, 1, "Count; the generic Page is left out")
	assert.Equal(t, "Count", fixture.Aliases[0].Name)
}

func TestGenerateSwift(t *testing.T) {
	t.Parallel()
	files := generateSwift(loadFixture(t))
	fixture := files["Fixture.swift"]
	for _, want := range []string{
		"public enum Fixture {}",
		"extension Fixture {\npublic enum Kind: Codable, Hashable, Sendable, CaseIterable {",
		// A case named for a keyword is escaped.
		"    case `default`\n",
		"    case unknown(String)\n",
		"public typealias Count = Int\n",
		// Other namespaces are named through the module, so Fixture.Shared
		// can't shadow Other.Shared.
		"    public var shared: ShishoAPI.Other.Shared\n",
		"    public var mine: Shared\n",
		"    public var color: ShishoAPI.Other.Color?\n",
		"    public var parent: Indirect<Widget>?\n",
		"    public var noTag: Bool\n",
		// Missing and null take the zero value; a pointer stays nil.
		"        self.kind = try c.decodeIfPresent(Kind.self, forKey: .kind) ?? .unknown(\"\")\n",
		"        self.children = try c.decodeIfPresent([Widget].self, forKey: .children) ?? []\n",
		"        self.maybe = try c.decodeIfPresent(String.self, forKey: .maybe)\n",
		"        case noTag = \"NoTag\"\n",
	} {
		assert.Contains(t, fixture, want)
	}
	assert.Contains(t, files["Other.swift"], "public struct Shared: Codable, Hashable, Sendable {")
	assert.Contains(t, files, "Support.swift")
}

func TestGenerateKotlin(t *testing.T) {
	t.Parallel()
	files := generateKotlin(loadFixture(t))
	fixture := files["Fixture.kt"]
	for _, want := range []string{
		"package com.shishobooks.shisho.api.fixture\n",
		"@Serializable(with = Kind.Serializer::class)\npublic sealed class Kind(public val raw: kotlin.String) {",
		"    public data object Default : Kind(\"default\")\n",
		"    public class Unknown(raw: kotlin.String) : Kind(raw) {",
		"public typealias Count = kotlin.Long\n",
		"    @SerialName(\"shared\") public val shared: com.shishobooks.shisho.api.other.Shared = com.shishobooks.shisho.api.other.Shared(),\n",
		"    @SerialName(\"kind\") public val kind: Kind = Kind.Unknown(\"\"),\n",
		"    @SerialName(\"maybe\") public val maybe: kotlin.String? = null,\n",
		"    @SerialName(\"children\") public val children: kotlin.collections.List<Widget> = kotlin.collections.emptyList(),\n",
		"    @SerialName(\"by_id\") public val byId: kotlin.collections.Map<kotlin.String, Widget> = kotlin.collections.emptyMap(),\n",
	} {
		assert.Contains(t, fixture, want)
	}
	assert.Contains(t, files["Support.kt"], "ignoreUnknownKeys = true")
}

func TestSync_WritesAndChecks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files := map[string]string{"A.swift": "a", "B.swift": "b"}

	stale, err := sync(filepath.Join(dir, "never-generated"), files, true)
	require.NoError(t, err)
	assert.Empty(t, stale, "a folder never generated has nothing stale")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "A.swift"), []byte("a"), 0o644))
	stale, err = sync(dir, files, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"B.swift"}, baseNames(stale), "a missing file is stale")

	_, err = sync(dir, files, false)
	require.NoError(t, err)
	stale, err = sync(dir, files, true)
	require.NoError(t, err)
	assert.Empty(t, stale)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "Old.swift"), []byte("x"), 0o644))
	files["A.swift"] = "changed"
	stale, err = sync(dir, files, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"A.swift", "Old.swift"}, baseNames(stale))

	_, err = sync(dir, files, false)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "Old.swift"))
	assert.True(t, os.IsNotExist(err), "a file no longer generated is removed")
}

func baseNames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	sort.Strings(out)
	return out
}
