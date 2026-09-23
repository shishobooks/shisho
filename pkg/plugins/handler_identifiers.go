package plugins

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/identifiers"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
)

// Identifier attribution (ADR 0006). Identifiers keep two layers: the
// aggregate file.IdentifierSource gates Scan replacement of the whole
// collection, and each entry's Source records where that entry came from.

// identifierField is one identifier object from Fields["identifiers"] with
// its strings trimmed. Intent is the raw per-entry "source" value, validated
// separately by validateSourceIntents; HasIntent distinguishes an absent
// "source" from one that is present but not a string.
type identifierField struct {
	Type, Value, Intent string
	HasIntent           bool
}

// identifierFields decodes the identifier objects in the untyped apply
// payload. Entries with the wrong shape are skipped; blank types and values
// are kept so callers can decide how to treat them.
func identifierFields(fields map[string]any) []identifierField {
	entries, ok := fields["identifiers"].([]any)
	if !ok {
		return nil
	}
	decoded := make([]identifierField, 0, len(entries))
	for _, entry := range entries {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		idType, _ := m["type"].(string)
		value, _ := m["value"].(string)
		rawIntent, hasIntent := m["source"]
		intent, _ := rawIntent.(string)
		decoded = append(decoded, identifierField{
			Type:      strings.TrimSpace(idType),
			Value:     strings.TrimSpace(value),
			Intent:    intent,
			HasIntent: hasIntent,
		})
	}
	return decoded
}

// validateIdentifierTypes rejects an identifier collection that lists a type
// twice. The store dedupes by type on insert, so without this check a
// duplicate would survive validation, the collection would be deleted, and
// only one of the duplicates would be reinserted. It runs before any field is
// persisted so the existing collection is untouched. Entries that
// convertFieldsToMetadata drops (blank type or value) do not count.
func validateIdentifierTypes(fields map[string]any) error {
	seen := make(map[string]struct{})
	for _, entry := range identifierFields(fields) {
		if entry.Type == "" || entry.Value == "" {
			continue
		}
		if _, dup := seen[entry.Type]; dup {
			return errcodes.ValidationError("duplicate identifier type: " + entry.Type)
		}
		seen[entry.Type] = struct{}{}
	}
	return nil
}

// applyIdentifiers replaces the file's identifier collection with proposed,
// attributing both layers by final value. It reports whether
// file.IdentifierSource changed and must be written.
//
// Per entry: an unchanged (type, normalized value) keeps its stored source; a
// changed or new entry takes its own intent (plugin source for a Proposal
// Acceptance, manual otherwise). The aggregate follows the field-level
// intent. A collection equal to the stored one is a no-op that preserves
// both layers without any delete or insert, and a clear nulls the aggregate
// so a later Scan may repopulate the collection.
func (h *handler) applyIdentifiers(ctx context.Context, file *models.File, proposed []mediafile.ParsedIdentifier, attr applyAttribution, overrides *ApplyOverrides) (bool, error) {
	var intents map[string]string
	if overrides != nil {
		intents = overrides.IdentifierIntents
	}
	toInsert := make([]*models.FileIdentifier, 0, len(proposed))
	for _, ident := range proposed {
		if ident.Type == "" || ident.Value == "" {
			continue
		}
		toInsert = append(toInsert, &models.FileIdentifier{
			FileID: file.ID,
			Type:   ident.Type,
			Value:  ident.Value,
			Source: attr.sourceForIntent(intents[ident.Type]),
		})
	}

	unchanged := identifiers.ReconcileSources(file.Identifiers, toInsert)
	if unchanged && (len(toInsert) > 0 || file.IdentifierSource == nil) {
		return false, nil
	}

	if !unchanged {
		if _, err := h.enrich.identStore.DeleteIdentifiersForFile(ctx, file.ID); err != nil {
			return false, errors.Wrap(err, "failed to delete identifiers")
		}
		if len(toInsert) > 0 {
			if err := h.enrich.identStore.BulkCreateFileIdentifiers(ctx, toInsert); err != nil {
				return false, errors.Wrap(err, "failed to bulk-create identifiers")
			}
		}
	}
	file.IdentifierSource = collectionSource(len(toInsert), attr.sourceFor("identifiers"))
	return true, nil
}
