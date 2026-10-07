package plugins

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests keep the TypeScript SDK in packages/plugin-sdk/ in step with
// what Go actually exposes to plugins (see "Plugin SDK" in
// pkg/plugins/AGENTS.md). The Go side is read at runtime wherever possible:
// the host APIs from a live goja runtime, the hook names from the property
// reads LoadPlugin makes, and the manifest fields from struct tags. A failure
// means one side changed without the other: update the matching .d.ts file
// (prefer additive, optional changes) or the Go code, whichever is wrong.

const sdkDir = "../../packages/plugin-sdk"

// sdkOnlyHostAPIs lists SDK host API members, as "namespace.method" or
// "member", that Go intentionally does not register. Empty: none.
var sdkOnlyHostAPIs = map[string]string{}

// goOnlyHostAPIs lists host API members Go registers that are intentionally
// left out of the SDK. Empty: none.
var goOnlyHostAPIs = map[string]string{}

var (
	tsBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	tsLineComment  = regexp.MustCompile(`(?m)^\s*//.*$`)
	tsInterface    = regexp.MustCompile(`(?ms)^export interface (\w+)[^{\n]*\{\n(.*?)^\}`)
	// Members sit at exactly two spaces of indentation, so the fields of
	// nested object types and multi-line parameter lists are skipped.
	tsMember     = regexp.MustCompile(`(?m)^  (?:readonly\s+)?(\w+)\??\s*[(:]`)
	tsMemberType = regexp.MustCompile(`(?m)^  (?:readonly\s+)?(\w+)\??\s*:\s*(\w+)\s*;`)
)

// readSDKInterfaces parses the interfaces in an SDK .d.ts file into their
// bodies, with comments removed.
func readSDKInterfaces(t *testing.T, file string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(sdkDir, file))
	require.NoError(t, err)
	return parseSDKInterfaces(string(data))
}

func parseSDKInterfaces(src string) map[string]string {
	src = tsBlockComment.ReplaceAllString(src, "")
	src = tsLineComment.ReplaceAllString(src, "")
	interfaces := map[string]string{}
	for _, m := range tsInterface.FindAllStringSubmatch(src, -1) {
		interfaces[m[1]] = m[2]
	}
	return interfaces
}

func interfaceMembers(body string) []string {
	var members []string
	for _, m := range tsMember.FindAllStringSubmatch(body, -1) {
		members = append(members, m[1])
	}
	return members
}

// sdkHostAPIs lists the members of ShishoHostAPI, expanding each member typed
// as another Shisho* interface into "namespace.method" entries.
func sdkHostAPIs(t *testing.T, interfaces map[string]string) []string {
	t.Helper()
	root, ok := interfaces["ShishoHostAPI"]
	require.True(t, ok, "host-api.d.ts must declare ShishoHostAPI")

	namespaceTypes := map[string]string{}
	for _, m := range tsMemberType.FindAllStringSubmatch(root, -1) {
		namespaceTypes[m[1]] = m[2]
	}

	var names []string
	for _, member := range interfaceMembers(root) {
		typ := namespaceTypes[member]
		body, isNamespace := interfaces[typ]
		if !isNamespace || !strings.HasPrefix(typ, "Shisho") {
			names = append(names, member)
			continue
		}
		for _, method := range interfaceMembers(body) {
			names = append(names, member+"."+method)
		}
	}
	sort.Strings(names)
	return names
}

// goHostAPIs injects the host APIs into a fresh runtime and walks the global
// shisho object: functions and plain values are listed by name, and each
// namespace object is listed as "namespace.method".
func goHostAPIs(t *testing.T) []string {
	t.Helper()
	rt := newTestRuntime("shisho", "sdk-sync")
	// dataDir is only defined when the runtime has a data directory.
	rt.dataDir = t.TempDir()
	require.NoError(t, InjectHostAPIs(rt, &mockConfigGetter{configs: map[string]*string{}}))

	shisho := rt.vm.Get("shisho").ToObject(rt.vm)
	var names []string
	for _, key := range shisho.Keys() {
		val := shisho.Get(key)
		if _, isFunc := goja.AssertFunction(val); isFunc {
			names = append(names, key)
			continue
		}
		obj, isObj := val.(*goja.Object)
		if !isObj {
			names = append(names, key)
			continue
		}
		for _, method := range obj.Keys() {
			names = append(names, key+"."+method)
		}
	}
	sort.Strings(names)
	return names
}

// missingFrom returns the entries of want that are absent from have and not
// covered by allowed.
func missingFrom(want, have []string, allowed map[string]string) []string {
	present := map[string]bool{}
	for _, h := range have {
		present[h] = true
	}
	var missing []string
	for _, w := range want {
		if _, ok := allowed[w]; !present[w] && !ok {
			missing = append(missing, w)
		}
	}
	return missing
}

func TestPluginSDK_HostAPIsMatchGo(t *testing.T) {
	t.Parallel()
	goNames := goHostAPIs(t)
	sdkNames := sdkHostAPIs(t, readSDKInterfaces(t, "host-api.d.ts"))
	require.NotEmpty(t, goNames)
	require.NotEmpty(t, sdkNames)

	assert.Empty(t, missingFrom(goNames, sdkNames, goOnlyHostAPIs),
		"Go registers these shisho.* host APIs but packages/plugin-sdk/host-api.d.ts does not declare them: add them to ShishoHostAPI or its namespace interface")
	assert.Empty(t, missingFrom(sdkNames, goNames, sdkOnlyHostAPIs),
		"packages/plugin-sdk/host-api.d.ts declares these shisho.* host APIs but Go does not register them in InjectHostAPIs: remove them from the SDK or implement them")
}

// goHookNames loads a plugin whose `plugin` global is a Proxy that records
// every property LoadPlugin reads from it, which is the set of hook names Go
// looks for, including lifecycle hooks such as onUninstalling.
func goHookNames(t *testing.T) []string {
	t.Helper()
	dir := t.TempDir()
	manifest := `{"manifestVersion": 1, "id": "sdk-sync", "name": "SDK Sync", "version": "1.0.0"}`
	mainJS := `var __reads = [];
var plugin = new Proxy({}, { get: function (target, prop) { __reads.push(String(prop)); return undefined; } });`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.js"), []byte(mainJS), 0o600))

	rt, err := LoadPlugin(dir, "shisho", "sdk-sync")
	require.NoError(t, err)

	var reads []string
	require.NoError(t, rt.vm.ExportTo(rt.vm.Get("__reads"), &reads))
	seen := map[string]bool{}
	var names []string
	for _, r := range reads {
		if !seen[r] {
			seen[r] = true
			names = append(names, r)
		}
	}
	sort.Strings(names)
	return names
}

func TestPluginSDK_HookNamesMatchGo(t *testing.T) {
	t.Parallel()
	goNames := goHookNames(t)
	require.NotEmpty(t, goNames)

	body, ok := readSDKInterfaces(t, "hooks.d.ts")["ShishoPlugin"]
	require.True(t, ok, "hooks.d.ts must declare ShishoPlugin")
	sdkNames := interfaceMembers(body)
	sort.Strings(sdkNames)

	assert.Equal(t, goNames, sdkNames,
		"the hooks LoadPlugin reads from the plugin object must match the members of ShishoPlugin in packages/plugin-sdk/hooks.d.ts")

	// HookTypes reports each hook that has a manifest capability; every one
	// of them must be a ShishoPlugin member.
	all := &Runtime{inputConverter: goja.Undefined(), fileParser: goja.Undefined(), outputGenerator: goja.Undefined(), metadataEnricher: goja.Undefined()}
	assert.Subset(t, sdkNames, all.HookTypes())
}

// manifestInterfaceNames maps Go manifest types to their SDK interface when
// the names differ.
var manifestInterfaceNames = map[string]string{
	"Manifest": "PluginManifest",
}

// collectManifestFields walks a manifest struct type and records the JSON
// field names of it and of every struct type it reaches, keyed by the SDK
// interface name.
func collectManifestFields(typ reflect.Type, out map[string][]string) {
	for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return
	}
	name := typ.Name()
	if mapped, ok := manifestInterfaceNames[name]; ok {
		name = mapped
	}
	if _, done := out[name]; done {
		return
	}
	out[name] = []string{}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := strings.Split(field.Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		out[name] = append(out[name], tag)
		collectManifestFields(field.Type, out)
	}
	sort.Strings(out[name])
}

func TestPluginSDK_ManifestMatchesGo(t *testing.T) {
	t.Parallel()
	goFields := map[string][]string{}
	collectManifestFields(reflect.TypeOf(Manifest{}), goFields)
	require.Contains(t, goFields, "PluginManifest")
	require.Contains(t, goFields, "ConfigField")

	interfaces := readSDKInterfaces(t, "manifest.d.ts")
	for iface, fields := range goFields {
		body, ok := interfaces[iface]
		if !assert.True(t, ok, "packages/plugin-sdk/manifest.d.ts must declare interface %s for the Go manifest type", iface) {
			continue
		}
		sdkFields := interfaceMembers(body)
		sort.Strings(sdkFields)
		assert.Equal(t, fields, sdkFields,
			"manifest interface %s in packages/plugin-sdk/manifest.d.ts must have the same fields as the Go struct's json tags", iface)
	}
}

var tsMetadataFieldUnion = regexp.MustCompile(`(?s)export type MetadataField =(.*?);`)
var tsStringLiteral = regexp.MustCompile(`"([^"]+)"`)

func TestPluginSDK_MetadataFieldsMatchGo(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join(sdkDir, "manifest.d.ts"))
	require.NoError(t, err)
	m := tsMetadataFieldUnion.FindStringSubmatch(string(data))
	require.NotNil(t, m, "manifest.d.ts must declare the MetadataField union")

	var sdkFields []string
	for _, lit := range tsStringLiteral.FindAllStringSubmatch(m[1], -1) {
		sdkFields = append(sdkFields, lit[1])
	}
	goFields := append([]string(nil), ValidMetadataFields...)
	sort.Strings(sdkFields)
	sort.Strings(goFields)
	assert.Equal(t, goFields, sdkFields,
		"MetadataField in packages/plugin-sdk/manifest.d.ts must list exactly ValidMetadataFields from pkg/plugins/manifest.go")
}

// TestPluginSDK_ParserCatchesDrift runs the SDK parsing against a crafted
// declaration file so the sync tests cannot pass by parsing nothing.
func TestPluginSDK_ParserCatchesDrift(t *testing.T) {
	t.Parallel()
	src := `/** Logging. */
export interface ShishoLog {
  /** Debug. */
  debug(msg: string): void;
  // info is commented out
  // info(msg: string): void;
  warn(
    msg: string,
  ): void;
}

export interface Nested {
  outer: {
    inner: string;
  };
}

export interface ShishoHostAPI {
  readonly dataDir: string;
  sleep(ms: number): void;
  log: ShishoLog;
}
`
	interfaces := parseSDKInterfaces(src)
	assert.Equal(t, []string{"outer"}, interfaceMembers(interfaces["Nested"]))

	sdkNames := sdkHostAPIs(t, interfaces)
	assert.Equal(t, []string{"dataDir", "log.debug", "log.warn", "sleep"}, sdkNames)

	goNames := []string{"dataDir", "log.debug", "log.info", "log.warn", "sleep"}
	assert.Equal(t, []string{"log.info"}, missingFrom(goNames, sdkNames, nil))
	assert.Empty(t, missingFrom(goNames, sdkNames, map[string]string{"log.info": "test"}))
}
