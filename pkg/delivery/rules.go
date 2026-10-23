// Package delivery holds Delivery Rules, which decide the File Type each of a
// book's Files reaches a reader as (ADR 0011), and the resolver that applies
// them.
package delivery

import (
	"fmt"
	"slices"

	"github.com/shishobooks/shisho/pkg/models"
)

// builtInOrder ranks the built-in File Types in Shisho's built-in rules.
// M4B is last, but as the only audiobook File Type it never competes with
// the others.
var builtInOrder = []string{
	models.FileTypeEPUB,
	models.FileTypeKePub,
	models.FileTypeAZW3,
	models.FileTypeMOBI,
	models.FileTypeCBZ,
	models.FileTypePDF,
	models.FileTypeM4B,
}

// BuiltInRules are Shisho's built-in Delivery Rules: every File Type
// delivered as itself. A KePub goes as itself too: it is a valid EPUB that
// other readers open, and a Kobo it is copied to by hand keeps what KePub is
// for. A user's default follows them until edited, so a change here reaches
// every user who never customized theirs.
func BuiltInRules() []Rule {
	rules := make([]Rule, 0, len(builtInOrder))
	for _, fileType := range builtInOrder {
		rules = append(rules, Rule{SourceFileType: fileType, DeliveredFileType: fileType})
	}
	return rules
}

// RulesError is the error Validate returns. It says why a customized list
// was refused, in a sentence a user can act on.
type RulesError struct {
	Reason string
}

func (e *RulesError) Error() string {
	return e.Reason
}

// Validate checks a customized list: it can't be empty, has at most one
// rule per source File Type, and each rule's source is a built-in File Type
// that reaches its delivered File Type.
func Validate(rules []Rule) error {
	if len(rules) == 0 {
		return &RulesError{Reason: "Delivery Rules need at least one rule."}
	}
	seen := map[string]bool{}
	for _, rule := range rules {
		if !models.IsBuiltInFileType(rule.SourceFileType) {
			return &RulesError{Reason: fmt.Sprintf("%q is not a File Type Shisho can deliver.", rule.SourceFileType)}
		}
		if seen[rule.SourceFileType] {
			return &RulesError{Reason: fmt.Sprintf("Delivery Rules can have only one rule for %s.", models.FileTypeLabel(rule.SourceFileType))}
		}
		seen[rule.SourceFileType] = true
		if _, ok := models.ConversionChain(rule.SourceFileType, rule.DeliveredFileType); !ok {
			return &RulesError{Reason: fmt.Sprintf("%s can't be delivered as %q.", models.FileTypeLabel(rule.SourceFileType), rule.DeliveredFileType)}
		}
	}
	return nil
}

// SourceFileTypes returns the source File Types of the rules that have a
// path to their delivered File Type, in rule order. A book is deliverable
// exactly when it has a main File of one of these types, so listings filter
// on them in SQL before paging. The result is never nil: listing filters
// read nil as "no filter", and rules that deliver nothing must match no
// book.
func SourceFileTypes(rules []Rule) []string {
	types := []string{}
	for _, rule := range rules {
		if _, ok := models.ConversionChain(rule.SourceFileType, rule.DeliveredFileType); ok && !slices.Contains(types, rule.SourceFileType) {
			types = append(types, rule.SourceFileType)
		}
	}
	return types
}
