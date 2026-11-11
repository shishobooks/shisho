package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// koreaderVersion is the KOReader release the harness runs. Its sync plugin
// changes between releases, and driver.lua calls plugin internals, so upgrading
// is a deliberate change: update the checksums, read the plugin's diff, and
// rerun `mise test:koreader`.
const koreaderVersion = "v2026.07.1"

// koreaderChecksums holds the release's SHA-256 for each Linux architecture.
var koreaderChecksums = map[string]string{
	"arm64":  "68b399a99a88bf3f036f5e8b67d043215a28e11d375cf0823900aef7b1523a0b",
	"x86_64": "299aadb28147a25e9432ced1214ea444a4184393b5ae97cf42402c8a61b1a1b0",
}

// containerImage is a plain Linux with no extra packages: the release bundles
// what it needs.
const containerImage = "debian:bookworm-slim"

// runTimeout bounds one KOReader run, so a hang fails with its output. It
// allows for pulling the image the first time.
const runTimeout = 5 * time.Minute

// resultMarker starts the driver's JSON line, among KOReader's own output.
const resultMarker = "@@RESULT@@"

//go:embed driver.lua
var driverScript []byte

//go:embed luatest.lua
var luatestScript []byte

// Harness runs commands against one KOReader home: its settings, its sign-in,
// its books, and the sidecars holding each book's position.
type Harness struct {
	// Dir holds the home. Everything under it lasts between commands.
	Dir string
	// ReleaseDir holds downloaded KOReader releases.
	ReleaseDir string
}

// Result is what a command printed, plus the raw JSON it came from.
type Result struct {
	OK         bool     `json:"ok"`
	Error      string   `json:"error,omitempty"`
	Hash       string   `json:"hash,omitempty"`
	Progress   string   `json:"progress,omitempty"`
	Percentage float64  `json:"percentage"`
	Page       int      `json:"page,omitempty"`
	PageCount  int      `json:"page_count,omitempty"`
	PageBased  bool     `json:"page_based"`
	Shown      []string `json:"-"`
	LoggedIn   bool     `json:"logged_in"`
	Prompted   string   `json:"prompted,omitempty"`
	Queued     int      `json:"queued"`
	Drained    int      `json:"drained"`
	Path       string   `json:"path,omitempty"`
	// Plugin is what the KOReader plugin holds after a plugin command, and
	// OPDS is KOReader's OPDS catalog list.
	Plugin *PluginReport `json:"plugin,omitempty"`
	OPDS   []OPDSServer  `json:"opds,omitempty"`
	Raw    json.RawMessage
}

// PluginReport is the KOReader plugin's state as its files hold it.
type PluginReport struct {
	Servers        []PluginServer `json:"servers"`
	InstalledFrom  string         `json:"installed_from"`
	Version        string         `json:"version"`
	Hash           string         `json:"hash"`
	SignInURL      *string        `json:"sign_in_url"`
	SignedIn       bool           `json:"signed_in"`
	Outcome        string         `json:"outcome"`
	Message        string         `json:"message"`
	Installed      bool           `json:"installed"`
	Restart        bool           `json:"restart"`
	PromptedUpdate string         `json:"prompted_update"`
	// Tools is what KOReader's Tools tab lists, in order, after plugin-menu,
	// and Menu what the Shisho menu does, with a submenu's items as
	// "Parent > Item".
	Tools []string `json:"tools"`
	Menu  []string `json:"menu"`
	Offer *struct {
		Server  string `json:"server"`
		Version string `json:"version"`
		Hash    string `json:"hash"`
	} `json:"offer"`
}

// PluginServer is one server the plugin signed in to.
type PluginServer struct {
	URL            string `json:"url"`
	Username       string `json:"username"`
	Credential     string `json:"credential"`
	DeviceID       string `json:"device_id"`
	DeviceName     string `json:"device_name"`
	NeedsLogin     bool   `json:"needs_login"`
	UpdateRequired bool   `json:"update_required"`
}

// OPDSServer is one catalog in KOReader's OPDS catalog list.
type OPDSServer struct {
	Title    string `json:"title"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// booksDir is where books the harness can open live on the host.
func (h *Harness) booksDir() string { return filepath.Join(h.Dir, "books") }

// Args are a driver command's arguments. File, Output, and Screenshot are
// host paths.
type Args struct {
	Command    string
	File       string
	Server     string
	Username   string
	Password   string
	URL        string
	Output     string
	Page       int
	Percent    *float64
	XPointer   string
	Mode       string
	View       string
	Confirm    bool
	Screenshot string
	// Online is whether KOReader has Wi-Fi in a plugin command, Code a
	// pairing code to sign in with, and From a server to install the plugin
	// from.
	Online bool
	Code   string
	From   string
}

// Run runs one command in a fresh container, as KOReader's unit tests start
// it, and returns what it printed.
func (h *Harness) Run(ctx context.Context, a Args) (*Result, error) {
	release, err := h.ensureRelease(ctx)
	if err != nil {
		return nil, err
	}
	home := filepath.Join(h.Dir, "home")
	screenshots := filepath.Join(h.Dir, "screenshots")
	for _, dir := range []string{home, h.booksDir(), screenshots} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	driver := filepath.Join(h.Dir, "driver.lua")
	if err := os.WriteFile(driver, driverScript, 0o600); err != nil {
		return nil, errors.WithStack(err)
	}

	payload := map[string]any{"command": a.Command}
	if a.File != "" {
		inContainer, err := h.bookInContainer(a.File)
		if err != nil {
			return nil, err
		}
		payload["file"] = inContainer
	}
	if a.Output != "" {
		payload["output"] = "/books/" + filepath.Base(a.Output)
	}
	if a.Server != "" {
		payload["server"] = containerURL(a.Server)
	}
	if a.URL != "" {
		payload["url"] = containerURL(a.URL)
	}
	if a.Username != "" {
		payload["username"] = a.Username
		payload["password"] = a.Password
	}
	if a.Page != 0 {
		payload["page"] = a.Page
	}
	if a.Percent != nil {
		payload["percent"] = *a.Percent
	}
	if a.XPointer != "" {
		payload["xpointer"] = a.XPointer
	}
	if a.Mode != "" {
		payload["mode"] = a.Mode
	}
	if a.View != "" {
		payload["view"] = a.View
	}
	if a.Confirm {
		payload["confirm"] = true
	}
	if a.Online {
		payload["online"] = true
	}
	if a.Code != "" {
		payload["code"] = a.Code
	}
	if a.From != "" {
		payload["from"] = containerURL(a.From)
	}
	shot := ""
	if a.Screenshot != "" {
		shot = "harness-shot.png"
		payload["screenshot"] = "/screenshots/" + shot
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	output, runErr := h.docker(ctx, release, driver, string(encoded),
		"-v", home+":/home/ko",
		"-v", h.booksDir()+":/books",
		"-v", screenshots+":/screenshots",
	)
	result, parseErr := parseResult(output)
	if parseErr != nil {
		tail := string(output)
		if len(tail) > 2000 {
			tail = tail[len(tail)-2000:]
		}
		return nil, errors.Errorf("koreader printed no result (%v, %v):\n%s", runErr, parseErr, tail)
	}
	if shot != "" && result.OK {
		if err := copyFile(filepath.Join(screenshots, shot), a.Screenshot); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// docker runs script with KOReader's luajit in a fresh container, as
// KOReader's unit tests start it, with mounts beyond KOReader's own, and
// returns everything it printed.
func (h *Harness) docker(ctx context.Context, release, script, argument string, mounts ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()
	dockerArgs := []string{
		"run", "--rm",
		// Files written to the mounts stay the caller's to delete.
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"-e", "KO_MULTIUSER=1",
		"-e", "HOME=/home/ko",
		"-e", "LC_ALL=C.UTF-8",
		"--add-host", "host.docker.internal:host-gateway",
		"-v", release + ":/opt/koreader:ro",
		"-v", script + ":/script.lua:ro",
	}
	dockerArgs = append(dockerArgs, mounts...)
	dockerArgs = append(dockerArgs, "-w", "/opt/koreader", containerImage, "./luajit", "/script.lua")
	if argument != "" {
		dockerArgs = append(dockerArgs, argument)
	}
	//nolint:gosec // A developer's own tool: its arguments come from its own flags.
	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return output.Bytes(), err
}

// LuaTestResult is what the plugin's Lua unit tests reported.
type LuaTestResult struct {
	OK     bool `json:"ok"`
	Passed int  `json:"passed"`
	Failed []struct {
		Name  string `json:"name"`
		Error string `json:"error"`
	} `json:"-"`
	Raw json.RawMessage `json:"-"`
}

// RunLuaTests runs the KOReader plugin's Lua unit tests: every spec file in
// pluginRoot's spec directory, against the plugin in its shisho.koplugin
// directory, with KOReader's own luajit and modules.
func (h *Harness) RunLuaTests(ctx context.Context, pluginRoot string) (*LuaTestResult, error) {
	release, err := h.ensureRelease(ctx)
	if err != nil {
		return nil, err
	}
	root, err := filepath.Abs(pluginRoot)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	home, err := os.MkdirTemp("", "koreader-luatest-")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer os.RemoveAll(home)
	script := filepath.Join(home, "luatest.lua")
	if err := os.WriteFile(script, luatestScript, 0o600); err != nil {
		return nil, errors.WithStack(err)
	}
	homeDir := filepath.Join(home, "home")
	if err := os.Mkdir(homeDir, 0o755); err != nil {
		return nil, errors.WithStack(err)
	}
	output, runErr := h.docker(ctx, release, script, "",
		"-v", homeDir+":/home/ko",
		"-v", filepath.Join(root, "shisho.koplugin")+":/plugin/shisho.koplugin:ro",
		"-v", filepath.Join(root, "spec")+":/spec:ro",
	)
	idx := bytes.LastIndex(output, []byte(resultMarker))
	if idx < 0 {
		return nil, errors.Errorf("the Lua tests printed no result (%v):\n%s", runErr, output)
	}
	line := output[idx+len(resultMarker):]
	if nl := bytes.IndexByte(line, '\n'); nl >= 0 {
		line = line[:nl]
	}
	result := &LuaTestResult{Raw: line}
	if err := json.Unmarshal(line, result); err != nil {
		return nil, errors.WithStack(err)
	}
	// An empty Lua table encodes as an object, so "failed" may be {}.
	var raw struct {
		Failed json.RawMessage `json:"failed"`
	}
	if err := json.Unmarshal(line, &raw); err == nil && len(raw.Failed) > 0 && raw.Failed[0] == '[' {
		if err := json.Unmarshal(raw.Failed, &result.Failed); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	return result, nil
}

// emptyTablesToArrays makes each of keys an empty array where Lua encoded
// an empty list as an empty object.
func emptyTablesToArrays(fields map[string]json.RawMessage, keys ...string) {
	for _, key := range keys {
		if value, ok := fields[key]; ok && strings.TrimSpace(string(value)) == "{}" {
			fields[key] = json.RawMessage("[]")
		}
	}
}

// pluginsDir is where KOReader looks for plugins in the harness's home.
func (h *Harness) pluginsDir() string {
	return filepath.Join(h.Dir, "home", ".config", "koreader", "plugins")
}

// InstallPlugin unzips a KOReader plugin zip into KOReader's plugins
// folder, replacing any plugin there, as a person does over USB.
func (h *Harness) InstallPlugin(data []byte) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return errors.WithStack(err)
	}
	plugins := h.pluginsDir()
	if err := os.MkdirAll(plugins, 0o755); err != nil {
		return errors.WithStack(err)
	}
	for _, f := range reader.File {
		if !strings.HasPrefix(f.Name, "shisho.koplugin/") || strings.Contains(f.Name, "..") {
			return errors.Errorf("unexpected file in the plugin zip: %s", f.Name)
		}
	}
	if err := os.RemoveAll(filepath.Join(plugins, "shisho.koplugin")); err != nil {
		return errors.WithStack(err)
	}
	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		dest := filepath.Join(plugins, filepath.FromSlash(f.Name))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return errors.WithStack(err)
		}
		if err := extract(f, dest); err != nil {
			return err
		}
	}
	return nil
}

func extract(f *zip.File, dest string) (err error) {
	in, err := f.Open()
	if err != nil {
		return errors.WithStack(err)
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = errors.WithStack(cerr)
		}
	}()
	//nolint:gosec // The plugin zip is Shisho's own, a few kilobytes.
	_, err = io.Copy(out, in)
	return errors.WithStack(err)
}

// parseResult finds the driver's JSON among KOReader's own output. An empty
// Lua table encodes as an object, so a "shown" with nothing in it needs care.
func parseResult(output []byte) (*Result, error) {
	idx := bytes.LastIndex(output, []byte(resultMarker))
	if idx < 0 {
		return nil, errors.New("no result line")
	}
	line := output[idx+len(resultMarker):]
	if nl := bytes.IndexByte(line, '\n'); nl >= 0 {
		line = line[:nl]
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, errors.WithStack(err)
	}
	emptyTablesToArrays(raw, "opds")
	if plugin, ok := raw["plugin"]; ok {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(plugin, &fields); err == nil {
			emptyTablesToArrays(fields, "servers", "tools", "menu")
			if raw["plugin"], err = json.Marshal(fields); err != nil {
				return nil, errors.WithStack(err)
			}
		}
	}
	line, err := json.Marshal(raw)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	result := &Result{}
	if err := json.Unmarshal(line, result); err != nil {
		return nil, errors.WithStack(err)
	}
	shown := []string{}
	if err := json.Unmarshal(raw["shown"], &shown); err != nil || shown == nil {
		shown = []string{}
	}
	result.Shown = shown
	encodedShown, err := json.Marshal(shown)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	raw["shown"] = encodedShown
	result.Raw, err = json.MarshalIndent(raw, "", "  ")
	return result, errors.WithStack(err)
}

// bookInContainer returns where the container sees path. A file from
// elsewhere, such as a library, is copied to the books directory byte for byte,
// since a copy hashes the same as the file.
func (h *Harness) bookInContainer(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", errors.WithStack(err)
	}
	if _, err := os.Stat(abs); err != nil {
		// A name in the books directory, like a download's.
		abs = filepath.Join(h.booksDir(), filepath.Base(path))
		if _, statErr := os.Stat(abs); statErr != nil {
			return "", errors.Wrapf(statErr, "no such book: %s", path)
		}
	}
	books, err := filepath.Abs(h.booksDir())
	if err != nil {
		return "", errors.WithStack(err)
	}
	if filepath.Dir(abs) != books {
		if err := os.MkdirAll(books, 0o755); err != nil {
			return "", errors.WithStack(err)
		}
		if err := copyFile(abs, filepath.Join(books, filepath.Base(abs))); err != nil {
			return "", err
		}
	}
	return "/books/" + filepath.Base(abs), nil
}

func copyFile(from, to string) (err error) {
	in, err := os.Open(from)
	if err != nil {
		return errors.WithStack(err)
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = errors.WithStack(cerr)
		}
	}()
	_, err = io.Copy(out, in)
	return errors.WithStack(err)
}

// containerURL points a URL on this machine at it from inside the container.
func containerURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "0.0.0.0", "::1":
		if port := u.Port(); port != "" {
			u.Host = "host.docker.internal:" + port
		} else {
			u.Host = "host.docker.internal"
		}
	}
	return u.String()
}

// ensureRelease returns the unpacked KOReader for the Docker server's
// architecture, downloading and verifying it the first time.
func (h *Harness) ensureRelease(ctx context.Context) (string, error) {
	arch, err := dockerArchitecture(ctx)
	if err != nil {
		return "", err
	}
	checksum, ok := koreaderChecksums[arch]
	if !ok {
		return "", errors.Errorf("no KOReader release for architecture %s", arch)
	}
	root, err := filepath.Abs(filepath.Join(h.ReleaseDir, koreaderVersion+"-"+arch))
	if err != nil {
		return "", errors.WithStack(err)
	}
	unpacked := filepath.Join(root, "lib", "koreader")
	if _, err := os.Stat(filepath.Join(unpacked, "luajit")); err == nil {
		return unpacked, nil
	}

	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		return "", errors.WithStack(err)
	}
	// Unpacked beside its final path, so an interrupted run leaves nothing
	// that looks complete.
	staging, err := os.MkdirTemp(filepath.Dir(root), "unpack-")
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer os.RemoveAll(staging)
	name := fmt.Sprintf("koreader-linux-%s-%s.tar.xz", arch, koreaderVersion)
	archive := filepath.Join(staging, name)
	fmt.Fprintf(os.Stderr, "downloading %s\n", name)
	source := fmt.Sprintf("https://github.com/koreader/koreader/releases/download/%s/%s", koreaderVersion, name)
	if err := download(ctx, source, archive, checksum); err != nil {
		return "", err
	}
	unpack := filepath.Join(staging, "release")
	if err := os.Mkdir(unpack, 0o755); err != nil {
		return "", errors.WithStack(err)
	}
	untar := exec.CommandContext(ctx, "tar", "-xJf", archive, "-C", unpack)
	if out, err := untar.CombinedOutput(); err != nil {
		return "", errors.Wrapf(err, "unpack KOReader: %s", out)
	}
	if err := os.Rename(unpack, root); err != nil {
		return "", errors.WithStack(err)
	}
	return unpacked, nil
}

func dockerArchitecture(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Arch}}").Output()
	if err != nil {
		return "", errors.Wrap(err, "the KOReader harness needs a running Docker")
	}
	switch arch := strings.TrimSpace(string(out)); arch {
	case "amd64":
		return "x86_64", nil
	default:
		return arch, nil
	}
}

func download(ctx context.Context, source, destination, checksum string) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return errors.WithStack(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.WithStack(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("download %s: %s", source, resp.Status)
	}
	out, err := os.Create(destination)
	if err != nil {
		return errors.WithStack(err)
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = errors.WithStack(cerr)
		}
	}()
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, hasher), resp.Body); err != nil {
		return errors.WithStack(err)
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); got != checksum {
		return errors.Errorf("download %s: checksum %s, want %s", source, got, checksum)
	}
	return nil
}

// percentArg parses a flag's percentage.
func percentArg(raw string) (*float64, error) {
	if raw == "" {
		return nil, nil
	}
	v, err := strconv.ParseFloat(strings.TrimSuffix(raw, "%"), 64)
	if err != nil {
		return nil, errors.Errorf("invalid percentage %q", raw)
	}
	return &v, nil
}
