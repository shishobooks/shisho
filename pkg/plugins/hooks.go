package plugins

import (
	"context"
	"math"
	"path/filepath"
	"strconv"
	"time"

	"github.com/dop251/goja"
	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/htmlutil"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
)

var errJSPanic = errors.New("JS runtime panicked")

// safeCallJS invokes a goja function with panic recovery. The goja runtime can
// panic on certain JS exceptions (e.g., nil pointer in handleThrow). This wrapper
// ensures plugin errors never crash the server.
//
// Note on cancellation: when the hook runner calls rt.vm.Interrupt() (see
// invokeHook), goja surfaces a *goja.InterruptedError as a normal return
// value from fn — it does not panic. That typed error flows through unwrapped
// so callers can detect it with errors.As. The recover() here is only for
// truly-unexpected runtime panics, which are still wrapped as errJSPanic.
func safeCallJS(fn goja.Callable, this goja.Value, args ...goja.Value) (result goja.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			// Belt-and-braces: in case goja ever panics with an
			// *InterruptedError instead of returning it, surface the typed
			// error so the caller can still distinguish cancellation from a
			// real JS panic.
			if ie, ok := r.(*goja.InterruptedError); ok {
				err = ie
				return
			}
			err = errors.Wrapf(errJSPanic, "%v", r)
		}
	}()
	return fn(this, args...)
}

// invokeHook runs fn while watching ctx for cancellation. If ctx becomes Done
// before fn returns, the goja VM is interrupted, causing any in-flight JS to
// throw *goja.InterruptedError at the next safepoint. The hook ctx is also
// stored on the runtime so blocking host APIs (sleep, http, ffmpeg, shell)
// can thread cancellation into their native Go calls — vm.Interrupt only
// fires between JS statements, not inside time.Sleep / http.Client.Do / etc.
//
// Callers must already hold rt.mu (the exclusive hook lock). invokeHook
// guarantees rt.vm.ClearInterrupt() runs before it returns, so the next hook
// invocation on the same runtime starts with a clean interrupt flag even if
// this one was cancelled.
func invokeHook(ctx context.Context, rt *Runtime, fn func()) {
	rt.hookCtx = ctx
	done := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			rt.vm.Interrupt(ctx.Err())
		case <-done:
		}
	}()
	defer func() {
		close(done)
		// Wait for the watcher so a racing Interrupt() cannot land after we
		// ClearInterrupt() and poison the next hook on this runtime.
		<-watcherDone
		rt.vm.ClearInterrupt()
		rt.hookCtx = nil
	}()
	fn()
}

// ConvertResult is the result of an input converter hook.
type ConvertResult struct {
	Success    bool
	TargetPath string
}

// RunInputConverter invokes a plugin's inputConverter.convert() hook.
func (m *Manager) RunInputConverter(ctx context.Context, rt *Runtime, sourcePath, targetDir string) (*ConvertResult, error) {
	if rt.inputConverter == nil {
		return nil, errors.New("plugin does not have an inputConverter hook")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Set up FSContext
	pluginDir := filepath.Join(m.pluginDir, rt.scope, rt.pluginID)
	fsCtx := NewFSContext(pluginDir, rt.dataDir, []string{sourcePath, targetDir}, rt.manifest.Capabilities.FileAccess)
	rt.SetFSContext(fsCtx)
	defer func() {
		rt.SetFSContext(nil)
		fsCtx.Cleanup() //nolint:errcheck
	}()

	// Get the convert method
	converterObj := rt.inputConverter.ToObject(rt.vm)
	convertVal := converterObj.Get("convert")
	if convertVal == nil || goja.IsUndefined(convertVal) {
		return nil, errors.New("inputConverter.convert is not defined")
	}
	convertFn, ok := goja.AssertFunction(convertVal)
	if !ok {
		return nil, errors.New("inputConverter.convert is not a function")
	}

	// Build the context argument
	contextObj := rt.vm.NewObject()
	contextObj.Set("sourcePath", sourcePath) //nolint:errcheck
	contextObj.Set("targetDir", targetDir)   //nolint:errcheck

	// Call the hook under a watcher that forwards ctx cancellation into the VM.
	var result goja.Value
	var callErr error
	invokeHook(ctx, rt, func() {
		result, callErr = safeCallJS(convertFn, goja.Undefined(), rt.vm.ToValue(contextObj))
	})
	if callErr != nil {
		return nil, errors.Wrap(callErr, "inputConverter.convert failed")
	}

	// Parse the result
	return parseConvertResult(rt.vm, result)
}

// RunFileParser invokes a plugin's fileParser.parse() hook.
func (m *Manager) RunFileParser(ctx context.Context, rt *Runtime, filePath, fileType string) (*mediafile.ParsedMetadata, error) {
	if rt.fileParser == nil {
		return nil, errors.New("plugin does not have a fileParser hook")
	}

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Set up FSContext
	pluginDir := filepath.Join(m.pluginDir, rt.scope, rt.pluginID)
	fsCtx := NewFSContext(pluginDir, rt.dataDir, []string{filePath}, rt.manifest.Capabilities.FileAccess)
	rt.SetFSContext(fsCtx)
	defer func() {
		rt.SetFSContext(nil)
		fsCtx.Cleanup() //nolint:errcheck
	}()

	// Get the parse method
	parserObj := rt.fileParser.ToObject(rt.vm)
	parseVal := parserObj.Get("parse")
	if parseVal == nil || goja.IsUndefined(parseVal) {
		return nil, errors.New("fileParser.parse is not defined")
	}
	parseFn, ok := goja.AssertFunction(parseVal)
	if !ok {
		return nil, errors.New("fileParser.parse is not a function")
	}

	// Build the context argument
	contextObj := rt.vm.NewObject()
	contextObj.Set("filePath", filePath) //nolint:errcheck
	contextObj.Set("fileType", fileType) //nolint:errcheck

	// Call the hook under a watcher that forwards ctx cancellation into the VM.
	var result goja.Value
	var callErr error
	invokeHook(ctx, rt, func() {
		result, callErr = safeCallJS(parseFn, goja.Undefined(), rt.vm.ToValue(contextObj))
	})
	if callErr != nil {
		return nil, errors.Wrap(callErr, "fileParser.parse failed")
	}

	// Parse the result
	md, err := parseParsedMetadata(rt.vm, result)
	if err != nil {
		return nil, err
	}

	// Default DataSource to this plugin's identity if not set by the plugin
	if md.DataSource == "" {
		md.DataSource = models.PluginDataSource(rt.scope, rt.pluginID)
	}

	return md, nil
}

// SearchResponse is the result of a metadata enricher's search() hook.
// Results are ParsedMetadata directly — no intermediate SearchResult type.
type SearchResponse struct {
	Results []mediafile.ParsedMetadata
}

// RunMetadataSearch invokes a plugin's metadataEnricher.search() hook.
func (m *Manager) RunMetadataSearch(ctx context.Context, rt *Runtime, searchCtx map[string]interface{}) (*SearchResponse, error) {
	if rt.metadataEnricher == nil {
		return nil, errors.New("plugin does not have a metadataEnricher hook")
	}

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Set up FSContext (no extra allowed paths for enrichers)
	pluginDir := filepath.Join(m.pluginDir, rt.scope, rt.pluginID)
	fsCtx := NewFSContext(pluginDir, rt.dataDir, nil, rt.manifest.Capabilities.FileAccess)
	rt.SetFSContext(fsCtx)
	defer func() {
		rt.SetFSContext(nil)
		fsCtx.Cleanup() //nolint:errcheck
	}()

	// Get the search method
	enricherObj := rt.metadataEnricher.ToObject(rt.vm)
	searchVal := enricherObj.Get("search")
	if searchVal == nil || goja.IsUndefined(searchVal) {
		return nil, errors.New("metadataEnricher.search is not defined")
	}
	searchFn, ok := goja.AssertFunction(searchVal)
	if !ok {
		return nil, errors.New("metadataEnricher.search is not a function")
	}

	// Call the hook under a watcher that forwards ctx cancellation into the VM.
	var result goja.Value
	var callErr error
	invokeHook(ctx, rt, func() {
		result, callErr = safeCallJS(searchFn, goja.Undefined(), rt.vm.ToValue(searchCtx))
	})
	if callErr != nil {
		return nil, errors.Wrap(callErr, "metadataEnricher.search failed")
	}

	// Parse the result
	return parseSearchResponse(rt.vm, result, rt.scope, rt.pluginID), nil
}

// RunOutputGenerator invokes a plugin's outputGenerator.generate() hook.
func (m *Manager) RunOutputGenerator(ctx context.Context, rt *Runtime, sourcePath, destPath string, bookCtx, fileCtx map[string]interface{}) error {
	if rt.outputGenerator == nil {
		return errors.New("plugin does not have an outputGenerator hook")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Set up FSContext
	pluginDir := filepath.Join(m.pluginDir, rt.scope, rt.pluginID)
	fsCtx := NewFSContext(pluginDir, rt.dataDir, []string{sourcePath, destPath}, rt.manifest.Capabilities.FileAccess)
	rt.SetFSContext(fsCtx)
	defer func() {
		rt.SetFSContext(nil)
		fsCtx.Cleanup() //nolint:errcheck
	}()

	// Get the generate method
	generatorObj := rt.outputGenerator.ToObject(rt.vm)
	generateVal := generatorObj.Get("generate")
	if generateVal == nil || goja.IsUndefined(generateVal) {
		return errors.New("outputGenerator.generate is not defined")
	}
	generateFn, ok := goja.AssertFunction(generateVal)
	if !ok {
		return errors.New("outputGenerator.generate is not a function")
	}

	// Build the context argument
	contextObj := rt.vm.NewObject()
	contextObj.Set("sourcePath", sourcePath) //nolint:errcheck
	contextObj.Set("destPath", destPath)     //nolint:errcheck
	contextObj.Set("book", bookCtx)          //nolint:errcheck
	contextObj.Set("file", fileCtx)          //nolint:errcheck

	// Call the hook under a watcher that forwards ctx cancellation into the VM.
	var callErr error
	invokeHook(ctx, rt, func() {
		_, callErr = safeCallJS(generateFn, goja.Undefined(), rt.vm.ToValue(contextObj))
	})
	if callErr != nil {
		return errors.Wrap(callErr, "outputGenerator.generate failed")
	}

	return nil
}

// RunFingerprint invokes a plugin's outputGenerator.fingerprint() hook.
func (m *Manager) RunFingerprint(rt *Runtime, bookCtx, fileCtx map[string]interface{}) (string, error) {
	if rt.outputGenerator == nil {
		return "", errors.New("plugin does not have an outputGenerator hook")
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Get the fingerprint method
	generatorObj := rt.outputGenerator.ToObject(rt.vm)
	fingerprintVal := generatorObj.Get("fingerprint")
	if fingerprintVal == nil || goja.IsUndefined(fingerprintVal) {
		return "", errors.New("outputGenerator.fingerprint is not defined")
	}
	fingerprintFn, ok := goja.AssertFunction(fingerprintVal)
	if !ok {
		return "", errors.New("outputGenerator.fingerprint is not a function")
	}

	// Build the context argument
	contextObj := rt.vm.NewObject()
	contextObj.Set("book", bookCtx) //nolint:errcheck
	contextObj.Set("file", fileCtx) //nolint:errcheck

	// Call the hook
	result, err := safeCallJS(fingerprintFn, goja.Undefined(), rt.vm.ToValue(contextObj))
	if err != nil {
		return "", errors.Wrap(err, "outputGenerator.fingerprint failed")
	}

	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		return "", errors.New("outputGenerator.fingerprint returned nil/undefined")
	}

	return result.String(), nil
}

// parseConvertResult maps a JS result object to ConvertResult.
func parseConvertResult(vm *goja.Runtime, val goja.Value) (*ConvertResult, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, errors.New("inputConverter.convert returned nil/undefined")
	}

	obj := val.ToObject(vm)
	result := &ConvertResult{}

	successVal := obj.Get("success")
	if successVal != nil && !goja.IsUndefined(successVal) {
		result.Success = successVal.ToBoolean()
	}

	targetPathVal := obj.Get("targetPath")
	if targetPathVal != nil && !goja.IsUndefined(targetPathVal) && !goja.IsNull(targetPathVal) {
		result.TargetPath = targetPathVal.String()
	}

	return result, nil
}

// parseSearchResponse maps a JS search result to SearchResponse.
// Each result is parsed directly into ParsedMetadata. The releaseDate field
// is parsed from "2006-01-02" or RFC3339 format strings into *time.Time.
// PluginScope and PluginID are set on each result for server-side tracking.
func parseSearchResponse(vm *goja.Runtime, val goja.Value, pluginScope, pluginID string) *SearchResponse {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return &SearchResponse{}
	}

	obj := val.ToObject(vm)
	resultsVal := obj.Get("results")
	if resultsVal == nil || goja.IsUndefined(resultsVal) || goja.IsNull(resultsVal) {
		return &SearchResponse{}
	}

	resultsObj := resultsVal.ToObject(vm)
	lengthVal := resultsObj.Get("length")
	if lengthVal == nil || goja.IsUndefined(lengthVal) {
		return &SearchResponse{}
	}
	length := int(lengthVal.ToInteger())

	results := make([]mediafile.ParsedMetadata, 0, length)
	for i := 0; i < length; i++ {
		itemVal := resultsObj.Get(intToString(i))
		if itemVal == nil || goja.IsUndefined(itemVal) || goja.IsNull(itemVal) {
			continue
		}
		itemObj := itemVal.ToObject(vm)

		md := mediafile.ParsedMetadata{
			Title:       getStringField(itemObj, "title"),
			Description: htmlutil.StripTags(getStringField(itemObj, "description")),
			Publisher:   getStringField(itemObj, "publisher"),
			Subtitle:    getStringField(itemObj, "subtitle"),
			Series:      getStringField(itemObj, "series"),
			Imprint:     getStringField(itemObj, "imprint"),
			URL:         getStringField(itemObj, "url"),
			CoverURL:    getStringField(itemObj, "coverUrl"),
			PluginScope: pluginScope,
			PluginID:    pluginID,
		}

		// releaseDate -> *time.Time (parse inline)
		releaseDateStr := getStringField(itemObj, "releaseDate")
		if releaseDateStr != "" {
			t, err := time.Parse("2006-01-02", releaseDateStr)
			if err != nil {
				t, err = time.Parse(time.RFC3339, releaseDateStr)
			}
			if err == nil {
				md.ReleaseDate = &t
			}
		}

		// seriesNumber -> *float64
		seriesNumVal := itemObj.Get("seriesNumber")
		if seriesNumVal != nil && !goja.IsUndefined(seriesNumVal) && !goja.IsNull(seriesNumVal) {
			f := seriesNumVal.ToFloat()
			md.SeriesNumber = &f
		}

		// confidence -> *float64 (0-1 score)
		confidenceVal := itemObj.Get("confidence")
		if confidenceVal != nil && !goja.IsUndefined(confidenceVal) && !goja.IsNull(confidenceVal) {
			c := confidenceVal.ToFloat()
			md.Confidence = &c
		}

		// coverPage -> *int (only accept finite non-negative integers; reject
		// negative, NaN, and Infinity to avoid broken previews downstream)
		coverPageVal := itemObj.Get("coverPage")
		if coverPageVal != nil && !goja.IsUndefined(coverPageVal) && !goja.IsNull(coverPageVal) {
			f := coverPageVal.ToFloat()
			if !math.IsNaN(f) && !math.IsInf(f, 0) && f >= 0 {
				cp := int(f)
				md.CoverPage = &cp
			}
		}

		// genres -> []string
		genresVal := itemObj.Get("genres")
		if genresVal != nil && !goja.IsUndefined(genresVal) && !goja.IsNull(genresVal) {
			md.Genres = parseStringArray(vm, genresVal)
		}

		// tags -> []string
		tagsVal := itemObj.Get("tags")
		if tagsVal != nil && !goja.IsUndefined(tagsVal) && !goja.IsNull(tagsVal) {
			md.Tags = parseStringArray(vm, tagsVal)
		}

		// narrators -> []string
		narratorsVal := itemObj.Get("narrators")
		if narratorsVal != nil && !goja.IsUndefined(narratorsVal) && !goja.IsNull(narratorsVal) {
			md.Narrators = parseStringArray(vm, narratorsVal)
		}

		// authors -> []ParsedAuthor
		authorsVal := itemObj.Get("authors")
		if authorsVal != nil && !goja.IsUndefined(authorsVal) && !goja.IsNull(authorsVal) {
			md.Authors = parseAuthors(vm, authorsVal)
		}

		// identifiers -> []ParsedIdentifier
		identifiersVal := itemObj.Get("identifiers")
		if identifiersVal != nil && !goja.IsUndefined(identifiersVal) && !goja.IsNull(identifiersVal) {
			md.Identifiers = parseIdentifiers(vm, identifiersVal)
		}

		// language -> *string (BCP 47 tag, normalized)
		languageStr := getStringField(itemObj, "language")
		if languageStr != "" {
			md.Language = mediafile.NormalizeLanguage(languageStr)
		}

		// abridged -> *bool
		abridgedVal := itemObj.Get("abridged")
		if abridgedVal != nil && !goja.IsUndefined(abridgedVal) && !goja.IsNull(abridgedVal) {
			b := abridgedVal.ToBoolean()
			md.Abridged = &b
		}

		results = append(results, md)
	}

	return &SearchResponse{Results: results}
}

// parseParsedMetadata maps a JS metadata object to mediafile.ParsedMetadata.
func parseParsedMetadata(vm *goja.Runtime, val goja.Value) (*mediafile.ParsedMetadata, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, errors.New("metadata result is nil/undefined")
	}

	obj := val.ToObject(vm)
	md := &mediafile.ParsedMetadata{}

	// String fields
	md.Title = getStringField(obj, "title")
	md.Subtitle = getStringField(obj, "subtitle")
	md.Series = getStringField(obj, "series")
	md.Description = htmlutil.StripTags(getStringField(obj, "description")) // strip HTML for clean display
	md.Publisher = getStringField(obj, "publisher")
	md.Imprint = getStringField(obj, "imprint")
	md.URL = getStringField(obj, "url")
	md.CoverMimeType = getStringField(obj, "coverMimeType")
	md.CoverURL = getStringField(obj, "coverUrl")
	md.DataSource = getStringField(obj, "dataSource")

	// seriesNumber -> *float64
	seriesNumVal := obj.Get("seriesNumber")
	if seriesNumVal != nil && !goja.IsUndefined(seriesNumVal) && !goja.IsNull(seriesNumVal) {
		f := seriesNumVal.ToFloat()
		md.SeriesNumber = &f
	}

	// authors -> []ParsedAuthor
	authorsVal := obj.Get("authors")
	if authorsVal != nil && !goja.IsUndefined(authorsVal) && !goja.IsNull(authorsVal) {
		md.Authors = parseAuthors(vm, authorsVal)
	}

	// narrators -> []string
	narratorsVal := obj.Get("narrators")
	if narratorsVal != nil && !goja.IsUndefined(narratorsVal) && !goja.IsNull(narratorsVal) {
		md.Narrators = parseStringArray(vm, narratorsVal)
	}

	// genres -> []string
	genresVal := obj.Get("genres")
	if genresVal != nil && !goja.IsUndefined(genresVal) && !goja.IsNull(genresVal) {
		md.Genres = parseStringArray(vm, genresVal)
	}

	// tags -> []string
	tagsVal := obj.Get("tags")
	if tagsVal != nil && !goja.IsUndefined(tagsVal) && !goja.IsNull(tagsVal) {
		md.Tags = parseStringArray(vm, tagsVal)
	}

	// releaseDate -> *time.Time (tries "2006-01-02" and RFC3339 formats for
	// consistency with parseSearchResponse)
	releaseDateVal := obj.Get("releaseDate")
	if releaseDateVal != nil && !goja.IsUndefined(releaseDateVal) && !goja.IsNull(releaseDateVal) {
		dateStr := releaseDateVal.String()
		if dateStr != "" {
			t, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				t, err = time.Parse(time.RFC3339, dateStr)
			}
			if err == nil {
				md.ReleaseDate = &t
			}
		}
	}

	// coverData -> []byte (from ArrayBuffer)
	coverDataVal := obj.Get("coverData")
	if coverDataVal != nil && !goja.IsUndefined(coverDataVal) && !goja.IsNull(coverDataVal) {
		md.CoverData = parseByteData(coverDataVal)
	}

	// coverPage -> *int (only accept finite non-negative integers; reject
	// negative, NaN, and Infinity to avoid broken previews downstream)
	coverPageVal := obj.Get("coverPage")
	if coverPageVal != nil && !goja.IsUndefined(coverPageVal) && !goja.IsNull(coverPageVal) {
		f := coverPageVal.ToFloat()
		if !math.IsNaN(f) && !math.IsInf(f, 0) && f >= 0 {
			cp := int(f)
			md.CoverPage = &cp
		}
	}

	// duration -> time.Duration (from seconds float)
	durationVal := obj.Get("duration")
	if durationVal != nil && !goja.IsUndefined(durationVal) && !goja.IsNull(durationVal) {
		seconds := durationVal.ToFloat()
		md.Duration = time.Duration(seconds * float64(time.Second))
	}

	// bitrateBps -> int
	bitrateVal := obj.Get("bitrateBps")
	if bitrateVal != nil && !goja.IsUndefined(bitrateVal) && !goja.IsNull(bitrateVal) {
		md.BitrateBps = int(bitrateVal.ToInteger())
	}

	// pageCount -> *int
	pageCountVal := obj.Get("pageCount")
	if pageCountVal != nil && !goja.IsUndefined(pageCountVal) && !goja.IsNull(pageCountVal) {
		pc := int(pageCountVal.ToInteger())
		md.PageCount = &pc
	}

	// identifiers -> []ParsedIdentifier
	identifiersVal := obj.Get("identifiers")
	if identifiersVal != nil && !goja.IsUndefined(identifiersVal) && !goja.IsNull(identifiersVal) {
		md.Identifiers = parseIdentifiers(vm, identifiersVal)
	}

	// chapters -> []ParsedChapter
	chaptersVal := obj.Get("chapters")
	if chaptersVal != nil && !goja.IsUndefined(chaptersVal) && !goja.IsNull(chaptersVal) {
		md.Chapters = parseChapters(vm, chaptersVal)
	}

	// language -> *string (BCP 47 tag, normalized)
	languageStr := getStringField(obj, "language")
	if languageStr != "" {
		md.Language = mediafile.NormalizeLanguage(languageStr)
	}

	// abridged -> *bool
	abridgedVal := obj.Get("abridged")
	if abridgedVal != nil && !goja.IsUndefined(abridgedVal) && !goja.IsNull(abridgedVal) {
		b := abridgedVal.ToBoolean()
		md.Abridged = &b
	}

	return md, nil
}

// getStringField reads a string property from a JS object, returning "" if absent/null/undefined.
func getStringField(obj *goja.Object, name string) string {
	val := obj.Get(name)
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return ""
	}
	return val.String()
}

// parseAuthors maps a JS array of {name, role} to []ParsedAuthor.
func parseAuthors(vm *goja.Runtime, val goja.Value) []mediafile.ParsedAuthor {
	obj := val.ToObject(vm)
	lengthVal := obj.Get("length")
	if lengthVal == nil || goja.IsUndefined(lengthVal) {
		return nil
	}
	length := int(lengthVal.ToInteger())

	authors := make([]mediafile.ParsedAuthor, 0, length)
	for i := 0; i < length; i++ {
		itemVal := obj.Get(intToString(i))
		if itemVal == nil || goja.IsUndefined(itemVal) || goja.IsNull(itemVal) {
			continue
		}
		itemObj := itemVal.ToObject(vm)
		authors = append(authors, mediafile.ParsedAuthor{
			Name: getStringField(itemObj, "name"),
			Role: getStringField(itemObj, "role"),
		})
	}
	return authors
}

// parseStringArray maps a JS array of strings to []string.
func parseStringArray(vm *goja.Runtime, val goja.Value) []string {
	obj := val.ToObject(vm)
	lengthVal := obj.Get("length")
	if lengthVal == nil || goja.IsUndefined(lengthVal) {
		return nil
	}
	length := int(lengthVal.ToInteger())

	result := make([]string, 0, length)
	for i := 0; i < length; i++ {
		itemVal := obj.Get(intToString(i))
		if itemVal == nil || goja.IsUndefined(itemVal) || goja.IsNull(itemVal) {
			continue
		}
		result = append(result, itemVal.String())
	}
	return result
}

// parseIdentifiers maps a JS array of {type, value} to []ParsedIdentifier.
func parseIdentifiers(vm *goja.Runtime, val goja.Value) []mediafile.ParsedIdentifier {
	obj := val.ToObject(vm)
	lengthVal := obj.Get("length")
	if lengthVal == nil || goja.IsUndefined(lengthVal) {
		return nil
	}
	length := int(lengthVal.ToInteger())

	identifiers := make([]mediafile.ParsedIdentifier, 0, length)
	for i := 0; i < length; i++ {
		itemVal := obj.Get(intToString(i))
		if itemVal == nil || goja.IsUndefined(itemVal) || goja.IsNull(itemVal) {
			continue
		}
		itemObj := itemVal.ToObject(vm)
		identifiers = append(identifiers, mediafile.ParsedIdentifier{
			Type:  getStringField(itemObj, "type"),
			Value: getStringField(itemObj, "value"),
		})
	}
	return identifiers
}

// parseChapters maps a JS array of chapter objects to []ParsedChapter.
func parseChapters(vm *goja.Runtime, val goja.Value) []mediafile.ParsedChapter {
	obj := val.ToObject(vm)
	lengthVal := obj.Get("length")
	if lengthVal == nil || goja.IsUndefined(lengthVal) {
		return nil
	}
	length := int(lengthVal.ToInteger())

	chapters := make([]mediafile.ParsedChapter, 0, length)
	for i := 0; i < length; i++ {
		itemVal := obj.Get(intToString(i))
		if itemVal == nil || goja.IsUndefined(itemVal) || goja.IsNull(itemVal) {
			continue
		}
		chapters = append(chapters, parseChapter(vm, itemVal))
	}
	return chapters
}

// parseChapter maps a single JS chapter object to ParsedChapter.
func parseChapter(vm *goja.Runtime, val goja.Value) mediafile.ParsedChapter {
	obj := val.ToObject(vm)
	ch := mediafile.ParsedChapter{
		Title: getStringField(obj, "title"),
	}

	startPageVal := obj.Get("startPage")
	if startPageVal != nil && !goja.IsUndefined(startPageVal) && !goja.IsNull(startPageVal) {
		sp := int(startPageVal.ToInteger())
		ch.StartPage = &sp
	}

	startTimestampVal := obj.Get("startTimestampMs")
	if startTimestampVal != nil && !goja.IsUndefined(startTimestampVal) && !goja.IsNull(startTimestampVal) {
		ts := startTimestampVal.ToInteger()
		ch.StartTimestampMs = &ts
	}

	hrefVal := obj.Get("href")
	if hrefVal != nil && !goja.IsUndefined(hrefVal) && !goja.IsNull(hrefVal) {
		href := hrefVal.String()
		ch.Href = &href
	}

	childrenVal := obj.Get("children")
	if childrenVal != nil && !goja.IsUndefined(childrenVal) && !goja.IsNull(childrenVal) {
		ch.Children = parseChapters(vm, childrenVal)
	}

	return ch
}

// parseByteData extracts bytes from a JS ArrayBuffer value.
func parseByteData(val goja.Value) []byte {
	exported := val.Export()
	switch v := exported.(type) {
	case goja.ArrayBuffer:
		return v.Bytes()
	case []byte:
		return v
	default:
		return nil
	}
}

// intToString converts an int to its string representation for JS array indexing.
func intToString(i int) string {
	return strconv.Itoa(i)
}

// RunOnUninstalling invokes a plugin's optional onUninstalling() lifecycle hook.
// This is called before uninstall to give the plugin a chance to clean up.
// Errors in the hook do not prevent uninstall.
func (m *Manager) RunOnUninstalling(rt *Runtime) {
	if rt.onUninstalling == nil {
		return
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Set up FSContext for the hook (plugin dir + data dir only)
	pluginDir := filepath.Join(m.pluginDir, rt.scope, rt.pluginID)
	fsCtx := NewFSContext(pluginDir, rt.dataDir, nil, rt.manifest.Capabilities.FileAccess)
	rt.SetFSContext(fsCtx)
	defer func() {
		rt.SetFSContext(nil)
		fsCtx.Cleanup() //nolint:errcheck
	}()

	// Call the hook — errors don't prevent uninstall
	log := logger.New()
	_, err := safeCallJS(rt.onUninstalling, goja.Undefined())
	if err != nil {
		log.Warn("onUninstalling hook failed", logger.Data{
			"plugin": rt.scope + "/" + rt.pluginID,
			"error":  err.Error(),
		})
	}
}
