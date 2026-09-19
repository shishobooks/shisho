package plugins

import (
	"fmt"
	"time"

	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
)

// Identify source attribution (ADR 0006). The browser reports, per selected
// field, whether the final value equals the Plugin Proposal (SourceIntent).
// The server owns the other half: a value that is semantically unchanged from
// stored state is a no-op that keeps its stored source, and only a changed
// value has its intent mapped to a canonical DataSource.

// applyAttribution maps validated SourceIntents to canonical sources for one
// apply call.
type applyAttribution struct {
	pluginSource string
	intents      map[string]string
}

func newApplyAttribution(pluginSource string, overrides *ApplyOverrides) applyAttribution {
	attr := applyAttribution{pluginSource: pluginSource}
	if overrides != nil {
		attr.intents = overrides.Intents
	}
	return attr
}

// sourceFor returns the canonical source for a changed field. Only an explicit
// "plugin" intent yields the plugin source. A missing entry is treated as
// "user": misattributing a plugin value as manual merely protects it from
// Scans, while the reverse lets a Scan overwrite an edit.
func (a applyAttribution) sourceFor(key string) string {
	if a.intents[key] == SourceIntentPlugin {
		return a.pluginSource
	}
	return models.DataSourceManual
}

func validSourceIntent(intent string) bool {
	return intent == SourceIntentPlugin || intent == SourceIntentUser
}

// validateSourceIntents rejects any intent outside the finite SourceIntent
// set, in both the payload's sources map and the reserved per-entry "source"
// on identifier objects, so no client-provided string can reach a source
// column.
func validateSourceIntents(payload *PluginApplyPayload) error {
	for key, intent := range payload.Sources {
		if !validSourceIntent(intent) {
			return errcodes.ValidationError(fmt.Sprintf("sources.%s must be one of: plugin, user", key))
		}
	}
	entries, _ := payload.Fields["identifiers"].([]any)
	for _, entry := range entries {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		raw, present := m["source"]
		if !present {
			continue
		}
		if intent, ok := raw.(string); !ok || !validSourceIntent(intent) {
			return errcodes.ValidationError("identifiers source must be one of: plugin, user")
		}
	}
	return nil
}

// applyOptional writes a canonicalized optional value and its source, where a
// nil value is an Explicit Clear. It reports false, leaving both untouched,
// when the value is a semantic no-op.
//
// A clear nulls the value and its source so a later Scan may repopulate the
// field. That includes a source left behind on an already-absent value:
// before ADR 0006 a clear kept the plugin source, which outranks embedded
// metadata, so clearing again has to be able to heal it.
func applyOptional[T any](value *T, stored **T, storedSource **string, source string, equal func(a, b T) bool) bool {
	if value == nil {
		if *stored == nil && *storedSource == nil {
			return false
		}
		*stored = nil
		*storedSource = nil
		return true
	}
	if *stored != nil && equal(*value, **stored) {
		return false
	}
	*stored = value
	*storedSource = &source
	return true
}

// applyOptionalValue is applyOptional for comparable values. Absence is nil
// rather than a zero value, which keeps a stored `false` distinct from absence
// for Abridged.
func applyOptionalValue[T comparable](value *T, stored **T, storedSource **string, source string) bool {
	return applyOptional(value, stored, storedSource, source, func(a, b T) bool { return a == b })
}

// applyOptionalString is applyOptionalValue for canonicalized strings, where
// an empty string is the Explicit Clear.
func applyOptionalString(value string, stored, storedSource **string, source string) bool {
	if value == "" {
		return applyOptionalValue(nil, stored, storedSource, source)
	}
	return applyOptionalValue(&value, stored, storedSource, source)
}

// sameCalendarDate reports whether two dates fall on the same UTC day.
// Identify edits release dates at day granularity, so a stored time of day
// must not turn an untouched date into a change.
func sameCalendarDate(a, b time.Time) bool {
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}
