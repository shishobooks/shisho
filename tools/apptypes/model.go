package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/tools/go/packages"
	"gopkg.in/yaml.v3"
)

// tygoConfig is the part of tygo.yaml the generator reads: which packages,
// and which of their files, hold API types.
type tygoConfig struct {
	Packages []struct {
		Path         string   `yaml:"path"`
		IncludeFiles []string `yaml:"include_files"`
		ExcludeFiles []string `yaml:"exclude_files"`
	} `yaml:"packages"`
}

// Kind is what a TypeRef holds.
type Kind int

const (
	KindString Kind = iota
	KindBool
	// KindInt is a Go int or int64: 64 bits on every platform the apps run.
	KindInt
	// KindInt32 is a Go integer of 32 bits or fewer.
	KindInt32
	KindDouble
	// KindNamed is a generated struct or alias, by Name.
	KindNamed
	// KindEnum is a generated enum, by Name.
	KindEnum
	KindArray
	// KindMap is a JSON object with string keys and Elem values.
	KindMap
	// KindJSON is any JSON value: an interface, a raw message, or a type
	// the generator can't express.
	KindJSON
)

// TypeRef is a field's type.
type TypeRef struct {
	Kind Kind
	// Name and Pkg name a generated struct, alias, or enum: its Go name and
	// the Go package that declares it, which the apps use as a namespace.
	Name string
	Pkg  string
	Elem *TypeRef
	// Optional is a Go pointer: the server may send null or leave it out.
	Optional bool
	// Indirect marks a field that would make a Swift struct contain itself,
	// so it's boxed.
	Indirect bool
}

// Field is one JSON field of a struct.
type Field struct {
	GoName   string
	JSONName string
	Type     TypeRef
}

// Struct is a generated struct.
type Struct struct {
	Name   string
	Doc    string
	Fields []*Field
}

// Enum is a set of string values with an unknown case.
type Enum struct {
	Name   string
	Doc    string
	Values []string
	// pkg is the Go package that declared it.
	pkg string
}

// Alias names another type.
type Alias struct {
	Name   string
	Doc    string
	Target TypeRef
}

// Package is one tygo package's types, in source order.
type Package struct {
	Name    string
	Structs []*Struct
	Enums   []*Enum
	Aliases []*Alias
}

// Model is every generated type.
type Model struct {
	Packages []*Package
	// structs are keyed by "pkg.Name".
	structs map[string]*Struct
}

var (
	emitPattern       = regexp.MustCompile(`^//tygo:emit export type (\w+) = (.*?);?\s*$`)
	typeofPattern     = regexp.MustCompile(`^typeof (\w+)$`)
	stringLitPattern  = regexp.MustCompile(`^"([^"]*)"$`)
	identifierPattern = regexp.MustCompile(`^\w+$`)
)

// loader turns tygo's packages into a Model.
type loader struct {
	root string
	// generated holds every declared Go type the generator writes.
	generated map[*types.TypeName]bool
	enums     map[string]*Enum
	// enumAliases are emits like `type DeliveredFileType = FileType`.
	enumAliases map[string]string
	model       *Model
}

// load reads tygo.yaml at configPath, whose directory is the module root,
// and returns every type its packages declare.
func load(configPath string) (*Model, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var cfg tygoConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, errors.Wrap(err, "parse tygo.yaml")
	}
	root, err := filepath.Abs(filepath.Dir(configPath))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	paths := make([]string, len(cfg.Packages))
	for i, p := range cfg.Packages {
		paths[i] = p.Path
	}
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedFiles,
		Dir:  root,
	}, paths...)
	if err != nil {
		return nil, errors.Wrap(err, "load packages")
	}
	byPath := map[string]*packages.Package{}
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			return nil, errors.Errorf("load %s: %v", p.PkgPath, p.Errors[0])
		}
		byPath[p.PkgPath] = p
	}

	l := &loader{root: root, generated: map[*types.TypeName]bool{}, enums: map[string]*Enum{}, enumAliases: map[string]string{}, model: &Model{structs: map[string]*Struct{}}}

	// The files of each package that tygo reads.
	type unit struct {
		pkg   *packages.Package
		files []*ast.File
	}
	var units []unit
	for _, entry := range cfg.Packages {
		pkg := byPath[entry.Path]
		if pkg == nil {
			return nil, errors.Errorf("package %s not loaded", entry.Path)
		}
		var files []*ast.File
		for _, file := range pkg.Syntax {
			base := filepath.Base(pkg.Fset.File(file.Pos()).Name())
			if strings.HasSuffix(base, "_test.go") {
				continue
			}
			if len(entry.IncludeFiles) > 0 && !slices.Contains(entry.IncludeFiles, base) {
				continue
			}
			if slices.Contains(entry.ExcludeFiles, base) {
				continue
			}
			files = append(files, file)
		}
		units = append(units, unit{pkg: pkg, files: files})
	}

	// Find every type first, so fields can refer to types in any package.
	// Each package is its own namespace in the apps, so a name declared in
	// two packages stays as it is in both.
	for _, u := range units {
		for _, file := range u.files {
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.TYPE {
					continue
				}
				for _, spec := range gen.Specs {
					ts := spec.(*ast.TypeSpec)
					if !ts.Name.IsExported() || ts.TypeParams != nil {
						continue
					}
					if obj, ok := u.pkg.TypesInfo.Defs[ts.Name].(*types.TypeName); ok {
						l.generated[obj] = true
					}
				}
			}
		}
	}

	// Enums come from tygo:emit comments, which name a union of constants or
	// string literals.
	for _, u := range units {
		for _, file := range u.files {
			for _, group := range file.Comments {
				for _, c := range group.List {
					m := emitPattern.FindStringSubmatch(strings.TrimSpace(c.Text))
					if m == nil {
						continue
					}
					l.addEmit(u.pkg, m[1], m[2])
				}
			}
		}
	}

	for _, u := range units {
		pkg := &Package{Name: u.pkg.Name}
		for _, file := range u.files {
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, spec := range gen.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || !ts.Name.IsExported() || ts.TypeParams != nil {
						continue
					}
					obj := u.pkg.TypesInfo.Defs[ts.Name].(*types.TypeName)
					doc := docText(ts.Doc, gen.Doc)
					name := obj.Name()
					switch underlying := obj.Type().Underlying().(type) {
					case *types.Struct:
						s := &Struct{Name: name, Doc: doc}
						l.addFields(s, underlying)
						pkg.Structs = append(pkg.Structs, s)
						l.model.structs[u.pkg.Name+"."+name] = s
					default:
						if enum, isEnum := l.enums[name]; isEnum && enum.pkg == u.pkg.Name {
							continue
						}
						target := l.typeRef(obj.Type().Underlying(), "")
						pkg.Aliases = append(pkg.Aliases, &Alias{Name: name, Doc: doc, Target: target})
					}
				}
			}
		}
		for _, enum := range l.enums {
			if enum.pkg == u.pkg.Name {
				pkg.Enums = append(pkg.Enums, enum)
			}
		}
		sort.Slice(pkg.Enums, func(i, j int) bool { return pkg.Enums[i].Name < pkg.Enums[j].Name })
		l.model.Packages = append(l.model.Packages, pkg)
	}
	markIndirect(l.model)
	return l.model, nil
}

// addEmit records the type a tygo:emit comment declares, when it's a union
// of string constants or literals, or another enum's name.
func (l *loader) addEmit(pkg *packages.Package, name, body string) {
	if identifierPattern.MatchString(body) {
		l.enumAliases[name] = body
		return
	}
	var values []string
	for _, term := range strings.Split(body, "|") {
		term = strings.TrimSpace(term)
		if m := typeofPattern.FindStringSubmatch(term); m != nil {
			obj, ok := pkg.Types.Scope().Lookup(m[1]).(*types.Const)
			if !ok || obj.Val().Kind() != constant.String {
				return
			}
			values = append(values, constant.StringVal(obj.Val()))
			continue
		}
		if m := stringLitPattern.FindStringSubmatch(term); m != nil {
			values = append(values, m[1])
			continue
		}
		return
	}
	l.enums[name] = &Enum{Name: name, Values: values, pkg: pkg.Name}
}

// enumNamed returns the enum a tstype names, following emitted aliases.
func (l *loader) enumNamed(name string) *Enum {
	for range 4 {
		if enum, ok := l.enums[name]; ok {
			return enum
		}
		target, ok := l.enumAliases[name]
		if !ok {
			return nil
		}
		name = target
	}
	return nil
}

// addFields adds st's JSON fields to s, flattening embedded structs as
// encoding/json does: of the fields with one JSON name, the shallowest wins,
// and several at the shallowest depth cancel each other out.
func (l *loader) addFields(s *Struct, st *types.Struct) {
	type candidate struct {
		field *Field
		depth int
	}
	var order []string
	byName := map[string][]candidate{}
	var walk func(st *types.Struct, depth int)
	walk = func(st *types.Struct, depth int) {
		for i := range st.NumFields() {
			field := st.Field(i)
			tag := reflect.StructTag(st.Tag(i))
			jsonName, jsonOpts, _ := strings.Cut(tag.Get("json"), ",")
			tstype, _, _ := strings.Cut(tag.Get("tstype"), ",")
			if jsonName == "-" && jsonOpts == "" || tag.Get("tstype") == "-" {
				continue
			}
			if field.Embedded() && jsonName == "" {
				// A struct from outside the module, like bun.BaseModel,
				// adds no API fields.
				if named, ok := deref(field.Type()).(*types.Named); ok && strings.HasPrefix(named.Obj().Pkg().Path(), "github.com/shishobooks/shisho/") {
					if inner, ok := named.Underlying().(*types.Struct); ok {
						walk(inner, depth+1)
					}
				}
				continue
			}
			if !field.Exported() {
				continue
			}
			if jsonName == "" {
				jsonName = field.Name()
			}
			if _, seen := byName[jsonName]; !seen {
				order = append(order, jsonName)
			}
			byName[jsonName] = append(byName[jsonName], candidate{
				field: &Field{GoName: field.Name(), JSONName: jsonName, Type: l.typeRef(field.Type(), tstype)},
				depth: depth,
			})
		}
	}
	walk(st, 0)
	for _, name := range order {
		candidates := byName[name]
		shallowest := candidates[0].depth
		for _, c := range candidates {
			shallowest = min(shallowest, c.depth)
		}
		var winners []*Field
		for _, c := range candidates {
			if c.depth == shallowest {
				winners = append(winners, c.field)
			}
		}
		if len(winners) == 1 {
			s.Fields = append(s.Fields, winners[0])
		}
	}
}

// typeRef maps a Go type to a generated one. tstype, the field's tstype tag,
// names an enum for a string field.
func (l *loader) typeRef(t types.Type, tstype string) TypeRef {
	tstype = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(tstype), "| null"))
	switch t := t.(type) {
	case *types.Pointer:
		ref := l.typeRef(t.Elem(), tstype)
		ref.Optional = true
		return ref
	case *types.Slice:
		if basic, ok := t.Elem().(*types.Basic); ok && basic.Kind() == types.Byte {
			return TypeRef{Kind: KindString}
		}
		elem := l.typeRef(t.Elem(), strings.TrimSuffix(tstype, "[]"))
		elem.Optional = false
		return TypeRef{Kind: KindArray, Elem: &elem}
	case *types.Array:
		elem := l.typeRef(t.Elem(), strings.TrimSuffix(tstype, "[]"))
		elem.Optional = false
		return TypeRef{Kind: KindArray, Elem: &elem}
	case *types.Map:
		elem := l.typeRef(t.Elem(), "")
		elem.Optional = false
		return TypeRef{Kind: KindMap, Elem: &elem}
	case *types.Named:
		obj := t.Obj()
		switch obj.Pkg().Path() + "." + obj.Name() {
		case "time.Time":
			return TypeRef{Kind: KindString}
		case "time.Duration":
			return TypeRef{Kind: KindInt}
		case "encoding/json.RawMessage":
			return TypeRef{Kind: KindJSON}
		}
		if l.generated[obj] {
			if enum := l.enumNamed(obj.Name()); enum != nil {
				return TypeRef{Kind: KindEnum, Name: enum.Name, Pkg: enum.pkg}
			}
			if _, isStruct := t.Underlying().(*types.Struct); isStruct {
				return TypeRef{Kind: KindNamed, Name: obj.Name(), Pkg: obj.Pkg().Name()}
			}
		}
		// A named basic type outside the generated packages, or one with no
		// alias, is its underlying type.
		return l.typeRef(t.Underlying(), tstype)
	case *types.Basic:
		if t.Info()&types.IsString != 0 {
			if enum := l.enumNamed(tstype); enum != nil {
				return TypeRef{Kind: KindEnum, Name: enum.Name, Pkg: enum.pkg}
			}
			return TypeRef{Kind: KindString}
		}
		switch t.Kind() { //nolint:exhaustive // Every other kind is any JSON value.
		case types.Bool:
			return TypeRef{Kind: KindBool}
		case types.Int, types.Int64, types.Uint, types.Uint64, types.Uint32:
			return TypeRef{Kind: KindInt}
		case types.Int8, types.Int16, types.Int32, types.Uint8, types.Uint16:
			return TypeRef{Kind: KindInt32}
		case types.Float32, types.Float64:
			return TypeRef{Kind: KindDouble}
		}
	}
	return TypeRef{Kind: KindJSON}
}

func deref(t types.Type) types.Type {
	if p, ok := t.(*types.Pointer); ok {
		return p.Elem()
	}
	return t
}

func docText(groups ...*ast.CommentGroup) string {
	for _, g := range groups {
		if g == nil {
			continue
		}
		var lines []string
		for _, line := range strings.Split(strings.TrimSpace(g.Text()), "\n") {
			if strings.HasPrefix(line, "tygo:") {
				continue
			}
			lines = append(lines, line)
		}
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}
	return ""
}

// markIndirect boxes the fields through which a struct would contain
// itself by value, which Swift can't lay out.
func markIndirect(m *Model) {
	// edges are by-value references: a struct field, optional or not.
	edges := func(s *Struct) []*Field {
		var out []*Field
		for _, f := range s.Fields {
			if f.Type.Kind == KindNamed {
				out = append(out, f)
			}
		}
		return out
	}
	key := func(ref TypeRef) string { return ref.Pkg + "." + ref.Name }
	// reaches reports whether from contains target by value.
	var reaches func(from, target string, seen map[string]bool) bool
	reaches = func(from, target string, seen map[string]bool) bool {
		if from == target {
			return true
		}
		if seen[from] {
			return false
		}
		seen[from] = true
		s := m.structs[from]
		if s == nil {
			return false
		}
		for _, f := range edges(s) {
			if f.Type.Indirect {
				continue
			}
			if reaches(key(f.Type), target, seen) {
				return true
			}
		}
		return false
	}
	for _, pkg := range m.Packages {
		for _, s := range pkg.Structs {
			for _, f := range edges(s) {
				if reaches(key(f.Type), pkg.Name+"."+s.Name, map[string]bool{}) {
					f.Type.Indirect = true
				}
			}
		}
	}
}

// pascal turns snake_case or a package name into PascalCase.
func pascal(s string) string {
	var b strings.Builder
	for _, part := range splitWords(s) {
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return b.String()
}

// camel turns snake_case into camelCase.
func camel(s string) string {
	p := pascal(s)
	if p == "" {
		return p
	}
	// Keep a leading acronym's first letter lower only.
	return strings.ToLower(p[:1]) + p[1:]
}

// splitWords splits on anything that isn't a letter or digit.
func splitWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	})
}

// caseName is an enum value's case name in lowerCamel, unique within the
// enum.
func caseNames(values []string, reserved ...string) []string {
	used := map[string]bool{}
	for _, r := range reserved {
		used[r] = true
	}
	out := make([]string, len(values))
	for i, v := range values {
		name := camel(v)
		if name == "" {
			name = "empty"
		}
		if name[0] >= '0' && name[0] <= '9' {
			name = "v" + name
		}
		base := name
		for n := 2; used[name]; n++ {
			name = base + strconv.Itoa(n)
		}
		used[name] = true
		out[i] = name
	}
	return out
}

// header is the generated files' first line.
func header(tool string) string {
	return fmt.Sprintf("// Code generated by %s from the Go API types (ADR 0021). DO NOT EDIT.\n", tool)
}
