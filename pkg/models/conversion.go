package models

import "strings"

// DeliveredFileTypeExtension returns the extension (no leading dot) of a
// File delivered as fileType: "kepub.epub" for a KePub, which Kobo and
// KOReader recognize by it, and the File Type itself for every built-in
// File Type.
func DeliveredFileTypeExtension(fileType string) string {
	if fileType == FileTypeKePub {
		return strings.TrimPrefix(kepubExtension, ".")
	}
	return fileType
}

// DeliveredFileTypeForExtension is the inverse of
// DeliveredFileTypeExtension. It returns false for an extension no
// delivered File Type has.
func DeliveredFileTypeForExtension(ext string) (string, bool) {
	for _, fileType := range BuiltInFileTypes {
		if DeliveredFileTypeExtension(fileType) == ext {
			return fileType, true
		}
	}
	return "", false
}

// FileTypeLabel is how a File Type is written for a reader: KePub, or the
// File Type in capitals.
func FileTypeLabel(fileType string) string {
	if fileType == FileTypeKePub {
		return "KePub"
	}
	return strings.ToUpper(fileType)
}

// ConversionStep is one Conversion: delivering a File of type From as type
// To. pkg/filegen holds the code that runs each step, and a test there
// checks that every step listed here has it.
type ConversionStep struct {
	From string
	To   string
}

// ConversionSteps are the Conversions Shisho can run. Chains of them are
// found automatically, so a new step makes every File Type that reaches its
// From reach its To too. The order breaks ties between chains of the same
// length and orders ReachableFileTypes.
var ConversionSteps = conversionGraph{
	{From: FileTypeEPUB, To: FileTypeKePub},
	{From: FileTypeCBZ, To: FileTypeKePub},
	{From: FileTypeCBZ, To: FileTypeEPUB},
	{From: FileTypeKePub, To: FileTypeEPUB},
}

type conversionGraph []ConversionStep

// paths walks the graph breadth first, starting at from, and returns, for
// each File Type reached, the step that first reached it, along with the
// File Types in the order they were reached.
func (g conversionGraph) paths(from string) (map[string]ConversionStep, []string) {
	via := map[string]ConversionStep{}
	order := []string{from}
	seen := map[string]bool{from: true}
	for i := 0; i < len(order); i++ {
		for _, step := range g {
			if step.From != order[i] || seen[step.To] {
				continue
			}
			seen[step.To] = true
			via[step.To] = step
			order = append(order, step.To)
		}
	}
	return via, order
}

// chain returns the shortest chain of steps leading from one File Type to
// another, and false when there is none. A File Type reaches itself with no
// steps.
func (g conversionGraph) chain(from, to string) ([]ConversionStep, bool) {
	if from == to {
		return nil, true
	}
	via, _ := g.paths(from)
	if _, ok := via[to]; !ok {
		return nil, false
	}
	var chain []ConversionStep
	for t := to; t != from; t = via[t].From {
		chain = append([]ConversionStep{via[t]}, chain...)
	}
	return chain, true
}

// reachable returns from followed by every File Type a chain reaches, nearest
// first.
func (g conversionGraph) reachable(from string) []string {
	_, order := g.paths(from)
	return order
}

// ConversionChain returns the steps that deliver a File of type from as type
// to, and false when no chain of Conversions reaches it.
func ConversionChain(from, to string) ([]ConversionStep, bool) {
	return ConversionSteps.chain(from, to)
}

// ReachableFileTypes returns the File Types a File of type from can be
// delivered as: from itself first, then every type a Conversion reaches.
func ReachableFileTypes(from string) []string {
	return ConversionSteps.reachable(from)
}
