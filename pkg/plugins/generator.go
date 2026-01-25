package plugins

import (
	"context"
	"sort"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
)

// PluginGenerator adapts a plugin's outputGenerator hook to the filegen.Generator interface.
type PluginGenerator struct {
	manager  *Manager
	scope    string
	pluginID string
	formatID string // The generator's format ID (e.g., "mobi")
}

// NewPluginGenerator creates a new PluginGenerator.
func NewPluginGenerator(manager *Manager, scope, pluginID, formatID string) *PluginGenerator {
	return &PluginGenerator{
		manager:  manager,
		scope:    scope,
		pluginID: pluginID,
		formatID: formatID,
	}
}

// Generate implements filegen.Generator by delegating to the plugin's outputGenerator.generate() hook.
func (g *PluginGenerator) Generate(ctx context.Context, srcPath, destPath string, book *models.Book, file *models.File) error {
	rt := g.manager.GetRuntime(g.scope, g.pluginID)
	if rt == nil {
		return errors.Errorf("plugin %s/%s not loaded", g.scope, g.pluginID)
	}

	bookCtx := BuildBookContext(book)
	fileCtx := BuildFileContext(file)

	return g.manager.RunOutputGenerator(ctx, rt, srcPath, destPath, bookCtx, fileCtx)
}

// SupportedType implements filegen.Generator.
func (g *PluginGenerator) SupportedType() string {
	return g.formatID
}

// Fingerprint returns the plugin's fingerprint string for cache invalidation.
func (g *PluginGenerator) Fingerprint(book *models.Book, file *models.File) (string, error) {
	rt := g.manager.GetRuntime(g.scope, g.pluginID)
	if rt == nil {
		return "", errors.Errorf("plugin %s/%s not loaded", g.scope, g.pluginID)
	}

	bookCtx := BuildBookContext(book)
	fileCtx := BuildFileContext(file)

	return g.manager.RunFingerprint(rt, bookCtx, fileCtx)
}

// BuildBookContext converts a Book model to a map for plugin context.
func BuildBookContext(book *models.Book) map[string]interface{} {
	if book == nil {
		return nil
	}

	ctx := map[string]interface{}{
		"id":    book.ID,
		"title": book.Title,
	}

	if book.Subtitle != nil {
		ctx["subtitle"] = *book.Subtitle
	}
	if book.Description != nil {
		ctx["description"] = *book.Description
	}

	// Authors
	if len(book.Authors) > 0 {
		authors := make([]*models.Author, len(book.Authors))
		copy(authors, book.Authors)
		sort.Slice(authors, func(i, j int) bool {
			return authors[i].SortOrder < authors[j].SortOrder
		})
		authorList := make([]map[string]interface{}, 0, len(authors))
		for _, a := range authors {
			if a.Person != nil {
				entry := map[string]interface{}{
					"name": a.Person.Name,
				}
				if a.Role != nil {
					entry["role"] = *a.Role
				}
				authorList = append(authorList, entry)
			}
		}
		ctx["authors"] = authorList
	}

	// Series
	if len(book.BookSeries) > 0 {
		series := make([]*models.BookSeries, len(book.BookSeries))
		copy(series, book.BookSeries)
		sort.Slice(series, func(i, j int) bool {
			return series[i].SortOrder < series[j].SortOrder
		})
		seriesList := make([]map[string]interface{}, 0, len(series))
		for _, s := range series {
			if s.Series != nil {
				entry := map[string]interface{}{
					"name": s.Series.Name,
				}
				if s.SeriesNumber != nil {
					entry["number"] = *s.SeriesNumber
				}
				seriesList = append(seriesList, entry)
			}
		}
		ctx["series"] = seriesList
	}

	// Genres
	if len(book.BookGenres) > 0 {
		genres := make([]string, 0, len(book.BookGenres))
		for _, bg := range book.BookGenres {
			if bg.Genre != nil {
				genres = append(genres, bg.Genre.Name)
			}
		}
		ctx["genres"] = genres
	}

	// Tags
	if len(book.BookTags) > 0 {
		tags := make([]string, 0, len(book.BookTags))
		for _, bt := range book.BookTags {
			if bt.Tag != nil {
				tags = append(tags, bt.Tag.Name)
			}
		}
		ctx["tags"] = tags
	}

	return ctx
}

// BuildFileContext converts a File model to a map for plugin context.
func BuildFileContext(file *models.File) map[string]interface{} {
	if file == nil {
		return nil
	}

	ctx := map[string]interface{}{
		"id":            file.ID,
		"filepath":      file.Filepath,
		"fileType":      file.FileType,
		"fileRole":      file.FileRole,
		"filesizeBytes": file.FilesizeBytes,
	}

	if file.Name != nil {
		ctx["name"] = *file.Name
	}
	if file.URL != nil {
		ctx["url"] = *file.URL
	}
	if file.Publisher != nil {
		ctx["publisher"] = file.Publisher.Name
	}
	if file.Imprint != nil {
		ctx["imprint"] = file.Imprint.Name
	}
	if file.ReleaseDate != nil {
		ctx["releaseDate"] = file.ReleaseDate.Format("2006-01-02")
	}

	// Narrators
	if len(file.Narrators) > 0 {
		narrators := make([]*models.Narrator, len(file.Narrators))
		copy(narrators, file.Narrators)
		sort.Slice(narrators, func(i, j int) bool {
			return narrators[i].SortOrder < narrators[j].SortOrder
		})
		narratorList := make([]string, 0, len(narrators))
		for _, n := range narrators {
			if n.Person != nil {
				narratorList = append(narratorList, n.Person.Name)
			}
		}
		ctx["narrators"] = narratorList
	}

	// Identifiers
	if len(file.Identifiers) > 0 {
		identifiers := make([]map[string]interface{}, 0, len(file.Identifiers))
		for _, id := range file.Identifiers {
			identifiers = append(identifiers, map[string]interface{}{
				"type":  id.Type,
				"value": id.Value,
			})
		}
		ctx["identifiers"] = identifiers
	}

	return ctx
}

// OutputFormatInfo describes a plugin-provided output format.
type OutputFormatInfo struct {
	ID          string
	Name        string
	SourceTypes []string
	Scope       string
	PluginID    string
}
