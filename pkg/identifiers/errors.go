package identifiers

import "github.com/shishobooks/shisho/pkg/errcodes"

// DuplicateTypeError is the 422 for an identifier collection that lists a
// type twice. The file update and plugin apply routes both reject it before
// writing, so they share one message.
func DuplicateTypeError(identifierType string) error {
	return errcodes.ValidationError("duplicate identifier type: " + identifierType)
}
