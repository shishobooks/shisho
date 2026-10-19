package models

const (
	// FileTypeKePub is Kobo's EPUB. Shisho never stores one as a File: it
	// exists only as a delivery, reached through Conversion.
	//tygo:emit export type DeliveredFileType = FileType | typeof FileTypeKePub;
	FileTypeKePub = "kepub"
)

// IsDeliveredFileType reports whether a File can be delivered as fileType: a
// built-in File Type or KePub.
func IsDeliveredFileType(fileType string) bool {
	return fileType == FileTypeKePub || IsBuiltInFileType(fileType)
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
