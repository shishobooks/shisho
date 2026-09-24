package identifiers

import "github.com/shishobooks/shisho/pkg/models"

// ReconcileSources gives each incoming identifier the per-entry source it
// should be stored with, and reports whether the incoming collection is the
// same set of identifiers as the existing one.
//
// An incoming entry whose (type, normalized value) already exists keeps the
// stored entry's source, because re-submitting an identifier says nothing new
// about where it came from. Every other entry keeps the source the caller
// assigned: manual for a hand-written value, or a plugin source for a
// Proposal Acceptance. Both sides are keyed on the normalized value so a
// stored value that predates normalization, or an incoming value the user
// typed with hyphens or in a different case, still matches.
//
// The Edit form and Identify share this helper so per-entry provenance is
// preserved identically by both endpoints (ADR 0006).
func ReconcileSources(existing, incoming []*models.FileIdentifier) (unchanged bool) {
	existingSources := make(map[string]string, len(existing))
	for _, id := range existing {
		existingSources[Key(id.Type, id.Value)] = id.Source
	}
	matched := 0
	for _, id := range incoming {
		if prev, ok := existingSources[Key(id.Type, id.Value)]; ok {
			id.Source = prev
			matched++
		}
	}
	// Types are unique within a collection, so matching every incoming entry
	// against a same-sized existing collection means the sets are equal.
	return matched == len(incoming) && matched == len(existingSources)
}
