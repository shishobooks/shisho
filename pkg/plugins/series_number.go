package plugins

import (
	"math"

	"github.com/dop251/goja"
	"github.com/shishobooks/shisho/pkg/models"
)

func parsePluginSeriesNumberGroup(obj *goja.Object) (*float64, *float64, *string) {
	start, startValid := optionalJSFloat(obj.Get("seriesNumber"))
	end, endValid := optionalJSFloat(obj.Get("seriesNumberEnd"))
	unit, unitValid := optionalJSString(obj.Get("seriesNumberUnit"))
	if !startValid || !endValid || !unitValid {
		return nil, nil, nil
	}
	return normalizePluginSeriesNumberGroup(start, end, unit)
}

func optionalJSFloat(value goja.Value) (*float64, bool) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, true
	}
	switch exported := value.Export().(type) {
	case int64:
		converted := float64(exported)
		return &converted, true
	case float64:
		return &exported, true
	default:
		return nil, false
	}
}

func optionalJSString(value goja.Value) (*string, bool) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, true
	}
	exported, ok := value.Export().(string)
	if !ok {
		return nil, false
	}
	return &exported, true
}

func seriesNumberGroupFromFields(fields map[string]any) (*float64, *float64, *string) {
	start, startValid := optionalFloatField(fields, "series_number")
	end, endValid := optionalFloatField(fields, "series_number_end")
	unit, unitValid := optionalStringField(fields, "series_number_unit")
	if !startValid || !endValid || !unitValid {
		return nil, nil, nil
	}
	return normalizePluginSeriesNumberGroup(start, end, unit)
}

func optionalFloatField(fields map[string]any, key string) (*float64, bool) {
	value, present := fields[key]
	if !present || value == nil {
		return nil, true
	}
	converted, ok := value.(float64)
	if !ok {
		return nil, false
	}
	return &converted, true
}

func optionalStringField(fields map[string]any, key string) (*string, bool) {
	value, present := fields[key]
	if !present || value == nil {
		return nil, true
	}
	converted, ok := value.(string)
	if !ok {
		return nil, false
	}
	return &converted, true
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
