package books

import (
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdfpages"
)

// PluginPageExtractor adapts ExtractCoverPageToFile to the plugins package's
// pageExtractor interface so plugin-provided cover_page values can be applied
// without the plugins package needing to depend on the page caches directly.
type PluginPageExtractor struct {
	cbzCache *cbzpages.Cache
	pdfCache *pdfpages.Cache
}

// NewPluginPageExtractor constructs a page extractor backed by the given
// cbzpages and pdfpages caches.
func NewPluginPageExtractor(cbzCache *cbzpages.Cache, pdfCache *pdfpages.Cache) *PluginPageExtractor {
	return &PluginPageExtractor{
		cbzCache: cbzCache,
		pdfCache: pdfCache,
	}
}

// ExtractCoverPage satisfies the plugins.pageExtractor interface by
// delegating to ExtractCoverPageToFile.
func (p *PluginPageExtractor) ExtractCoverPage(file *models.File, bookFilepath string, page int, log logger.Logger) (string, string, error) {
	return ExtractCoverPageToFile(file, bookFilepath, page, p.cbzCache, p.pdfCache, log)
}
