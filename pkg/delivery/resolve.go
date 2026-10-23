package delivery

import (
	"slices"

	"github.com/shishobooks/shisho/pkg/models"
)

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

// SourceMatch is one way a book can be delivered as a File Type: it has a
// main File of SourceFileType and no main File of any type in Unless.
type SourceMatch struct {
	SourceFileType string
	Unless         []string
}

// DeliveredAs returns the ways a book can be delivered as fileType under
// rules, so a listing can filter on the File Type a reader receives before
// paging. A book Resolve delivers some File of as fileType meets exactly one
// of them: in each category, the earliest rule with a chain whose source File
// Type the book has goes, so a rule delivering fileType counts only for
// books with none of the earlier rules' source File Types in its category.
// The result is never nil, so a File Type nothing is delivered as matches no
// book. Keep it in step with resolveCategory.
func DeliveredAs(rules []Rule, fileType string) []SourceMatch {
	matches := []SourceMatch{}
	var earlierEbooks, earlierAudiobooks []string
	for _, rule := range rules {
		if _, ok := models.ConversionChain(rule.SourceFileType, rule.DeliveredFileType); !ok {
			continue
		}
		var earlier *[]string
		switch {
		case models.IsEbookFileType(rule.SourceFileType):
			earlier = &earlierEbooks
		case models.IsAudiobookFileType(rule.SourceFileType):
			earlier = &earlierAudiobooks
		default:
			continue
		}
		if rule.DeliveredFileType == fileType {
			matches = append(matches, SourceMatch{SourceFileType: rule.SourceFileType, Unless: slices.Clone(*earlier)})
		}
		*earlier = append(*earlier, rule.SourceFileType)
	}
	return matches
}
