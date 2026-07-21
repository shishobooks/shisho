package worker

import (
	"github.com/shishobooks/shisho/pkg/identifiers"
	"github.com/shishobooks/shisho/pkg/mediafile"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sidecar"
)

// shouldUpdateScalar determines if a scalar field should be updated based on priority rules.
// Returns true if the new value should replace the existing value.
// When forceRefresh is true, priority checks are bypassed (but empty values are still skipped).
func shouldUpdateScalar(newValue, existingValue, newSource, existingSource string, forceRefresh bool) bool {
	// Never update with empty new value
	if newValue == "" {
		return false
	}

	// Skip if values are the same — unless forceRefresh is true and the source
	// changed, in which case we must update to keep source attribution correct.
	// A stale source (e.g., "plugin:foo" after a reset) would cause future
	// priority-based scans to make wrong overwrite decisions.
	if newValue == existingValue {
		if forceRefresh && newSource != existingSource {
			return true
		}
		return false
	}

	// Force refresh bypasses priority checks
	if forceRefresh {
		return true
	}

	// Empty existing source is treated as filepath priority (backwards compatibility)
	if existingSource == "" {
		existingSource = models.DataSourceFilepath
	}

	newPriority := models.GetDataSourcePriority(newSource)
	existingPriority := models.GetDataSourcePriority(existingSource)

	// Higher or equal priority wins when new value is non-empty and different
	return newPriority <= existingPriority
}

// shouldUpdateRelationship determines if a relationship (authors, series, etc.) should be updated.
// Returns true if the new items should replace the existing items.
// When forceRefresh is true, priority checks are bypassed (but empty items are still skipped).
func shouldUpdateRelationship(newItems, existingItems []string, newSource, existingSource string, forceRefresh bool) bool {
	// Never update with empty new items
	if len(newItems) == 0 {
		return false
	}

	// Skip if items are the same — unless forceRefresh is true and the source
	// changed, in which case we must update to keep source attribution correct.
	if equalStringSlices(newItems, existingItems) {
		if forceRefresh && newSource != existingSource {
			return true
		}
		return false
	}

	// Force refresh bypasses priority checks
	if forceRefresh {
		return true
	}

	// Empty existing source is treated as filepath priority (backwards compatibility)
	if existingSource == "" {
		existingSource = models.DataSourceFilepath
	}

	newPriority := models.GetDataSourcePriority(newSource)
	existingPriority := models.GetDataSourcePriority(existingSource)

	// Higher or equal priority wins when new items are non-empty and different
	return newPriority <= existingPriority
}

// equalStringSlices compares two string slices for equality (order matters).
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// shouldApplySidecarScalar determines if a sidecar scalar value should be applied.
// Sidecars have higher priority than file metadata and can override it.
// When forceRefresh is true, sidecars are skipped entirely - the embedded file metadata wins.
func shouldApplySidecarScalar(newValue, existingValue, existingSource string, forceRefresh bool) bool {
	// Force refresh skips sidecars - embedded file metadata should win
	if forceRefresh {
		return false
	}

	// Never apply empty values
	if newValue == "" {
		return false
	}

	// Never apply if values are the same
	if newValue == existingValue {
		return false
	}

	// Empty existing source is treated as filepath priority
	if existingSource == "" {
		existingSource = models.DataSourceFilepath
	}

	// Sidecar has its own priority level, higher than file metadata
	sidecarPriority := models.GetDataSourcePriority(models.DataSourceSidecar)
	existingPriority := models.GetDataSourcePriority(existingSource)

	return sidecarPriority < existingPriority
}

// shouldApplySidecarRelationship determines if a sidecar relationship should be applied.
// Sidecars have higher priority than file metadata and can override it.
// When forceRefresh is true, sidecars are skipped entirely - the embedded file metadata wins.
func shouldApplySidecarRelationship(newItems, existingItems []string, existingSource string, forceRefresh bool) bool {
	// Force refresh skips sidecars - embedded file metadata should win
	if forceRefresh {
		return false
	}

	// Never apply empty items
	if len(newItems) == 0 {
		return false
	}

	// Never apply if items are the same
	if equalStringSlices(newItems, existingItems) {
		return false
	}

	// Empty existing source is treated as filepath priority
	if existingSource == "" {
		existingSource = models.DataSourceFilepath
	}

	// Sidecar has its own priority level, higher than file metadata
	sidecarPriority := models.GetDataSourcePriority(models.DataSourceSidecar)
	existingPriority := models.GetDataSourcePriority(existingSource)

	return sidecarPriority < existingPriority
}

func shouldUpdateParsedSeries(incoming *mediafile.ParsedMetadata, existing []*models.BookSeries, existingSource string, forceRefresh bool) bool {
	if incoming == nil || incoming.Series == "" {
		return false
	}
	newSource := incoming.SourceForField("series")
	if len(existing) != 1 || existing[0].Series == nil || existing[0].Series.Name != incoming.Series {
		existingNames := make([]string, 0, len(existing))
		for _, membership := range existing {
			if membership.Series != nil {
				existingNames = append(existingNames, membership.Series.Name)
			}
		}
		return shouldUpdateRelationship([]string{incoming.Series}, existingNames, newSource, existingSource, forceRefresh)
	}

	// A partially present or invalid external group must never mutate storage.
	groupPresent := incoming.SeriesNumber != nil || incoming.SeriesNumberEnd != nil || incoming.SeriesNumberUnit != nil
	if groupPresent && !validSeriesNumberGroup(incoming.SeriesNumber, incoming.SeriesNumberEnd, incoming.SeriesNumberUnit) {
		return false
	}
	current := existing[0]
	groupMatches := equalFloatPointers(incoming.SeriesNumber, current.SeriesNumber) &&
		equalFloatPointers(incoming.SeriesNumberEnd, current.SeriesNumberEnd) &&
		equalStringPointers(incoming.SeriesNumberUnit, current.SeriesNumberUnit)
	if groupMatches {
		return forceRefresh && newSource != existingSource
	}
	if !groupPresent {
		return forceRefresh
	}
	if forceRefresh {
		return true
	}
	if existingSource == "" {
		existingSource = models.DataSourceFilepath
	}
	return models.GetDataSourcePriority(newSource) <= models.GetDataSourcePriority(existingSource)
}

func shouldApplySeriesSidecar(incoming []sidecar.SeriesMetadata, existing []*models.BookSeries, existingSource string, forceRefresh bool) bool {
	if forceRefresh || len(incoming) == 0 {
		return false
	}
	for _, membership := range incoming {
		groupPresent := membership.Number != nil || membership.NumberEnd != nil || membership.Unit != nil
		if groupPresent && !validSeriesNumberGroup(membership.Number, membership.NumberEnd, membership.Unit) {
			return false
		}
	}
	if seriesSidecarMatches(incoming, existing) {
		return false
	}
	if existingSource == "" {
		existingSource = models.DataSourceFilepath
	}
	return models.GetDataSourcePriority(models.DataSourceSidecar) < models.GetDataSourcePriority(existingSource)
}

func seriesSidecarMatches(incoming []sidecar.SeriesMetadata, existing []*models.BookSeries) bool {
	if len(incoming) != len(existing) {
		return false
	}
	for i := range incoming {
		if existing[i].Series == nil || incoming[i].Name != existing[i].Series.Name ||
			!equalFloatPointers(incoming[i].Number, existing[i].SeriesNumber) ||
			!equalFloatPointers(incoming[i].NumberEnd, existing[i].SeriesNumberEnd) ||
			!equalStringPointers(incoming[i].Unit, existing[i].SeriesNumberUnit) {
			return false
		}
	}
	return true
}

func equalFloatPointers(a, b *float64) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func equalStringPointers(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func applySeriesNumberUnit(metadata *mediafile.ParsedMetadata, unit, source string) {
	if metadata == nil || unit == "" || metadata.SeriesNumber == nil || metadata.SeriesNumberUnit != nil {
		return
	}
	if metadata.SourceForField("series") != source {
		return
	}
	u := unit
	metadata.SeriesNumberUnit = &u
}

// fileIdentifierKeys returns canonical comparison keys for a set of stored
// file identifiers. Centralizing this in one helper (rather than inlining the
// key construction at each diff site) ensures that both sides of a scan diff
// — stored DB values and freshly-parsed values — use the same format, so a
// rescan against cosmetically-different-but-semantically-identical parser
// output (e.g. hyphenated ISBNs, mixed-case ASINs) does not thrash
// delete+insert on every run. Paired with parsedIdentifierKeys below.
func fileIdentifierKeys(ids []*models.FileIdentifier) []string {
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, identifiers.Key(id.Type, id.Value))
	}
	return keys
}

// parsedIdentifierKeys returns canonical comparison keys for a set of freshly
// parsed identifiers emitted by a file parser. Must use the same format as
// fileIdentifierKeys so the two sides of a scan diff compare correctly.
func parsedIdentifierKeys(ids []mediafile.ParsedIdentifier) []string {
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, identifiers.Key(id.Type, id.Value))
	}
	return keys
}

// sidecarIdentifierKeys returns canonical comparison keys for a set of
// sidecar-sourced identifiers. Same contract as parsedIdentifierKeys, but
// for the distinct sidecar.IdentifierMetadata type.
func sidecarIdentifierKeys(ids []sidecar.IdentifierMetadata) []string {
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, identifiers.Key(id.Type, id.Value))
	}
	return keys
}

// appendIfMissing appends items to the slice only if they're not already present.
// Used to avoid duplicating columns when sidecar and metadata both want to update the same field.
func appendIfMissing(slice []string, items ...string) []string {
	for _, item := range items {
		found := false
		for _, s := range slice {
			if s == item {
				found = true
				break
			}
		}
		if !found {
			slice = append(slice, item)
		}
	}
	return slice
}
