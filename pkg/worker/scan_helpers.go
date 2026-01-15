package worker

import "github.com/shishobooks/shisho/pkg/models"

// shouldUpdateScalar determines if a scalar field should be updated based on priority rules.
// Returns true if the new value should replace the existing value.
func shouldUpdateScalar(newValue, existingValue, newSource, existingSource string) bool {
	// Empty existing source is treated as filepath priority (backwards compatibility)
	if existingSource == "" {
		existingSource = models.DataSourceFilepath
	}

	// Never update with empty new value
	if newValue == "" {
		return false
	}

	// Never update if values are the same
	if newValue == existingValue {
		return false
	}

	newPriority := models.DataSourcePriority[newSource]
	existingPriority := models.DataSourcePriority[existingSource]

	// Higher or equal priority wins when new value is non-empty and different
	return newPriority <= existingPriority
}

// shouldUpdateRelationship determines if a relationship (authors, series, etc.) should be updated.
// Returns true if the new items should replace the existing items.
func shouldUpdateRelationship(newItems, existingItems []string, newSource, existingSource string) bool {
	// Empty existing source is treated as filepath priority (backwards compatibility)
	if existingSource == "" {
		existingSource = models.DataSourceFilepath
	}

	// Never update with empty new items
	if len(newItems) == 0 {
		return false
	}

	// Never update if items are the same
	if equalStringSlices(newItems, existingItems) {
		return false
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
