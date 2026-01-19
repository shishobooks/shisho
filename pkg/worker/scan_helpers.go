package worker

import "github.com/shishobooks/shisho/pkg/models"

// shouldUpdateScalar determines if a scalar field should be updated based on priority rules.
// Returns true if the new value should replace the existing value.
// When forceRefresh is true, priority checks are bypassed (but empty values are still skipped).
func shouldUpdateScalar(newValue, existingValue, newSource, existingSource string, forceRefresh bool) bool {
	// Never update with empty new value
	if newValue == "" {
		return false
	}

	// Never update if values are the same (even with forceRefresh - no point changing source if value is identical)
	if newValue == existingValue {
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

	newPriority := models.DataSourcePriority[newSource]
	existingPriority := models.DataSourcePriority[existingSource]

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

	// Never update if items are the same (even with forceRefresh - no point changing source if items are identical)
	if equalStringSlices(newItems, existingItems) {
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

	newPriority := models.DataSourcePriority[newSource]
	existingPriority := models.DataSourcePriority[existingSource]

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
	sidecarPriority := models.DataSourcePriority[models.DataSourceSidecar]
	existingPriority := models.DataSourcePriority[existingSource]

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
	sidecarPriority := models.DataSourcePriority[models.DataSourceSidecar]
	existingPriority := models.DataSourcePriority[existingSource]

	return sidecarPriority < existingPriority
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
