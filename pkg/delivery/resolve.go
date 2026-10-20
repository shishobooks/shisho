package delivery

import "github.com/shishobooks/shisho/pkg/models"

// Delivery is one File that goes to a reader: the File, the File Type it is
// delivered as, and the Conversions that reach that type (none when it is
// delivered as itself).
type Delivery struct {
	File              *models.File
	DeliveredFileType string
	Chain             []models.ConversionStep
}

// Result is what a list of Delivery Rules delivers from one book, per
// category. Each category is decided on its own, so a book with an ebook and
// an audiobook delivers the best of each.
type Result struct {
	Ebook     []Delivery
	Audiobook []Delivery
}

// All returns every Delivery, ebooks first.
func (r Result) All() []Delivery {
	return append(append([]Delivery{}, r.Ebook...), r.Audiobook...)
}

// Resolve is the one place that decides what a book delivers under a list of
// Delivery Rules. In each category, the earliest rule matching one of the
// book's main Files wins, and every File it matches goes, so Files tied on
// that rule all go. A rule matches a File of its source File Type only when
// a chain of Conversions reaches its delivered File Type; a rule with no
// path counts as unmatched. A File Type with no rule is never delivered, and
// neither is a supplement.
func Resolve(rules []Rule, book *models.Book) Result {
	return Result{
		Ebook:     resolveCategory(rules, book.Files, models.IsEbookFileType),
		Audiobook: resolveCategory(rules, book.Files, models.IsAudiobookFileType),
	}
}

func resolveCategory(rules []Rule, files []*models.File, inCategory func(string) bool) []Delivery {
	for _, rule := range rules {
		if !inCategory(rule.SourceFileType) {
			continue
		}
		chain, ok := models.ConversionChain(rule.SourceFileType, rule.DeliveredFileType)
		if !ok {
			continue
		}
		var deliveries []Delivery
		for _, file := range files {
			if file.FileRole == models.FileRoleSupplement || file.FileType != rule.SourceFileType {
				continue
			}
			deliveries = append(deliveries, Delivery{File: file, DeliveredFileType: rule.DeliveredFileType, Chain: chain})
		}
		if len(deliveries) > 0 {
			return deliveries
		}
	}
	return nil
}
