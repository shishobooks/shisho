package kobo

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

// nativeKoboResourcesJSON is a snapshot of the upstream Kobo store
// /v1/initialization "Resources" map. It is sourced verbatim from Komga's
// nativeKoboResources fallback (which is itself a snapshot of the real Kobo
// API response). We never proxy /v1/initialization to the real Kobo store
// because we do not hold a valid Kobo OAuth token to authenticate the call.
// The proxied response would otherwise carry firmware-update prompts and
// other URLs that a stock Kobo device follows and aborts on when they fail.
//
// All keys are returned to the device; we override the three image URL keys
// to point at our cover handler so cover thumbnails load locally.
//
//go:embed native_resources.json
var nativeKoboResourcesJSON []byte

var (
	nativeKoboResourcesOnce sync.Once
	nativeKoboResourcesMap  map[string]interface{}
	nativeKoboResourcesErr  error
)

// loadNativeKoboResources parses the embedded JSON exactly once. The parsed
// map is treated as immutable; callers that need to mutate must clone first
// via cloneNativeKoboResources.
func loadNativeKoboResources() (map[string]interface{}, error) {
	nativeKoboResourcesOnce.Do(func() {
		var m map[string]interface{}
		if err := json.Unmarshal(nativeKoboResourcesJSON, &m); err != nil {
			nativeKoboResourcesErr = fmt.Errorf("parse native kobo resources: %w", err)
			return
		}
		nativeKoboResourcesMap = m
	})
	return nativeKoboResourcesMap, nativeKoboResourcesErr
}

// buildInitResources returns a fresh map of init Resources with the three
// image URL keys pointed at our base URL. baseURL is expected to already be
// scoped (e.g. "https://shisho.local/kobo/<key>/library/1") so the templates
// resolve to our cover handler regardless of which Kobo scope the device is
// using.
func buildInitResources(baseURL string) (map[string]interface{}, error) {
	src, err := loadNativeKoboResources()
	if err != nil {
		return nil, err
	}
	out := make(map[string]interface{}, len(src)+3)
	for k, v := range src {
		out[k] = v
	}
	out["image_host"] = baseURL
	out["image_url_template"] = baseURL + "/v1/books/{ImageId}/thumbnail/{Width}/{Height}/false/image.jpg"
	out["image_url_quality_template"] = baseURL + "/v1/books/{ImageId}/thumbnail/{Width}/{Height}/{Quality}/{IsGreyscale}/image.jpg"
	return out, nil
}
