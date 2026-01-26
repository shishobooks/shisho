package plugins

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	"github.com/dop251/goja"
	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/htmlutil"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
)

// ConvertResult is the result of an input converter hook.
type ConvertResult struct {
	Success    bool
	TargetPath string
}

// EnrichmentResult is the result of a metadata enricher hook.
type EnrichmentResult struct {
	Modified bool
	Metadata *mediafile.ParsedMetadata // nil if Modified is false
}

// RunInputConverter invokes a plugin's inputConverter.convert() hook.
func (m *Manager) RunInputConverter(ctx context.Context, rt *Runtime, sourcePath, targetDir string) (*ConvertResult, error) {
	if rt.inputConverter == nil {
		return nil, errors.New("plugin does not have an inputConverter hook")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	_ = ctx // reserved for future cancellation support

	rt.mu.RLock()
	defer rt.mu.RUnlock()

	// Set up FSContext
	pluginDir := filepath.Join(m.pluginDir, rt.scope, rt.pluginID)
	fsCtx := NewFSContext(pluginDir, []string{sourcePath, targetDir}, rt.manifest.Capabilities.FileAccess)
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

	// Call the hook
	result, err := convertFn(goja.Undefined(), rt.vm.ToValue(contextObj))
	if err != nil {
		return nil, errors.Wrap(err, "inputConverter.convert failed")
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
	_ = ctx // reserved for future cancellation support

	rt.mu.RLock()
	defer rt.mu.RUnlock()

	// Set up FSContext
	pluginDir := filepath.Join(m.pluginDir, rt.scope, rt.pluginID)
	fsCtx := NewFSContext(pluginDir, []string{filePath}, rt.manifest.Capabilities.FileAccess)
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

	// Call the hook
	result, err := parseFn(goja.Undefined(), rt.vm.ToValue(contextObj))
	if err != nil {
		return nil, errors.Wrap(err, "fileParser.parse failed")
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

// RunMetadataEnricher invokes a plugin's metadataEnricher.enrich() hook.
func (m *Manager) RunMetadataEnricher(ctx context.Context, rt *Runtime, enrichCtx map[string]interface{}) (*EnrichmentResult, error) {
	if rt.metadataEnricher == nil {
		return nil, errors.New("plugin does not have a metadataEnricher hook")
	}

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
	_ = ctx // reserved for future cancellation support

	rt.mu.RLock()
	defer rt.mu.RUnlock()

	// Set up FSContext (no extra allowed paths for enrichers)
	pluginDir := filepath.Join(m.pluginDir, rt.scope, rt.pluginID)
	fsCtx := NewFSContext(pluginDir, nil, rt.manifest.Capabilities.FileAccess)
	rt.SetFSContext(fsCtx)
	defer func() {
		rt.SetFSContext(nil)
		fsCtx.Cleanup() //nolint:errcheck
	}()

	// Get the enrich method
	enricherObj := rt.metadataEnricher.ToObject(rt.vm)
	enrichVal := enricherObj.Get("enrich")
	if enrichVal == nil || goja.IsUndefined(enrichVal) {
		return nil, errors.New("metadataEnricher.enrich is not defined")
	}
	enrichFn, ok := goja.AssertFunction(enrichVal)
	if !ok {
		return nil, errors.New("metadataEnricher.enrich is not a function")
	}

	// Call the hook
	result, err := enrichFn(goja.Undefined(), rt.vm.ToValue(enrichCtx))
	if err != nil {
		return nil, errors.Wrap(err, "metadataEnricher.enrich failed")
	}

	// Parse the result
	return parseEnrichmentResult(rt.vm, result)
}

// RunOutputGenerator invokes a plugin's outputGenerator.generate() hook.
func (m *Manager) RunOutputGenerator(ctx context.Context, rt *Runtime, sourcePath, destPath string, bookCtx, fileCtx map[string]interface{}) error {
	if rt.outputGenerator == nil {
		return errors.New("plugin does not have an outputGenerator hook")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	_ = ctx // reserved for future cancellation support

	rt.mu.RLock()
	defer rt.mu.RUnlock()

	// Set up FSContext
	pluginDir := filepath.Join(m.pluginDir, rt.scope, rt.pluginID)
	fsCtx := NewFSContext(pluginDir, []string{sourcePath, destPath}, rt.manifest.Capabilities.FileAccess)
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

	// Call the hook
	_, err := generateFn(goja.Undefined(), rt.vm.ToValue(contextObj))
	if err != nil {
		return errors.Wrap(err, "outputGenerator.generate failed")
	}

	return nil
}

// RunFingerprint invokes a plugin's outputGenerator.fingerprint() hook.
func (m *Manager) RunFingerprint(rt *Runtime, bookCtx, fileCtx map[string]interface{}) (string, error) {
	if rt.outputGenerator == nil {
		return "", errors.New("plugin does not have an outputGenerator hook")
	}

	rt.mu.RLock()
	defer rt.mu.RUnlock()

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
	result, err := fingerprintFn(goja.Undefined(), rt.vm.ToValue(contextObj))
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

// parseEnrichmentResult maps a JS result object to EnrichmentResult.
func parseEnrichmentResult(vm *goja.Runtime, val goja.Value) (*EnrichmentResult, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, errors.New("metadataEnricher.enrich returned nil/undefined")
	}

	obj := val.ToObject(vm)
	result := &EnrichmentResult{}

	modifiedVal := obj.Get("modified")
	if modifiedVal != nil && !goja.IsUndefined(modifiedVal) {
		result.Modified = modifiedVal.ToBoolean()
	}

	if !result.Modified {
		return result, nil
	}

	metadataVal := obj.Get("metadata")
	if metadataVal == nil || goja.IsUndefined(metadataVal) || goja.IsNull(metadataVal) {
		return result, nil
	}

	metadata, err := parseParsedMetadata(vm, metadataVal)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse enrichment metadata")
	}
	result.Metadata = metadata

	return result, nil
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

	// releaseDate -> *time.Time (ISO 8601 string)
	releaseDateVal := obj.Get("releaseDate")
	if releaseDateVal != nil && !goja.IsUndefined(releaseDateVal) && !goja.IsNull(releaseDateVal) {
		dateStr := releaseDateVal.String()
		if dateStr != "" {
			t, err := time.Parse(time.RFC3339, dateStr)
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

	// coverPage -> *int
	coverPageVal := obj.Get("coverPage")
	if coverPageVal != nil && !goja.IsUndefined(coverPageVal) && !goja.IsNull(coverPageVal) {
		cp := int(coverPageVal.ToInteger())
		md.CoverPage = &cp
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
