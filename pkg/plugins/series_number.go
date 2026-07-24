package plugins

import (
	"math"

	"github.com/dop251/goja"
	"github.com/shishobooks/shisho/pkg/models"
)

func parsePluginSeriesNumberGroup(obj *goja.Object) (*float64, *float64, *string) {
	start := optionalJSFloat(obj.Get("seriesNumber"))
	end := optionalJSFloat(obj.Get("seriesNumberEnd"))
	unit := optionalJSString(obj.Get("seriesNumberUnit"))
	return normalizePluginSeriesNumberGroup(start, end, unit)
}

func optionalJSFloat(value goja.Value) *float64 {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil
	}
	converted := value.ToFloat()
	return &converted
}

func optionalJSString(value goja.Value) *string {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil
	}
	converted := value.String()
	return &converted
}

func seriesNumberGroupFromFields(fields map[string]any) (*float64, *float64, *string) {
	var start *float64
	if value, ok := fields["series_number"].(float64); ok {
		start = &value
	}
	var end *float64
	if value, ok := fields["series_number_end"].(float64); ok {
		end = &value
	}
	var unit *string
	if value, ok := fields["series_number_unit"].(string); ok {
		unit = &value
	}
	return normalizePluginSeriesNumberGroup(start, end, unit)
}

func normalizePluginSeriesNumberGroup(start, end *float64, unit *string) (*float64, *float64, *string) {
	if start == nil || math.IsNaN(*start) || math.IsInf(*start, 0) {
		return nil, nil, nil
	}
	if end != nil && (math.IsNaN(*end) || math.IsInf(*end, 0) || *end <= *start) {
		return nil, nil, nil
	}
	if unit != nil && *unit != models.SeriesNumberUnitVolume && *unit != models.SeriesNumberUnitChapter {
		return nil, nil, nil
	}
	return start, end, unit
}
