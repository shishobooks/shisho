package plugins

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAvailablePluginResponse_JSONFields locks the JSON wire format for the
// /plugins/available response. The frontend depends on these exact field names.
func TestAvailablePluginResponse_JSONFields(t *testing.T) {
	t.Parallel()

	resp := availablePluginResponse{
		Scope:       "shisho",
		ID:          "example",
		Name:        "Example",
		Overview:    "ov",
		Description: "desc",
		Homepage:    "https://example.com",
		ImageURL:    "https://example.com/logo.png",
		IsOfficial:  true,
		Versions:    nil,
		Compatible:  true,
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, "https://example.com/logo.png", decoded["imageUrl"], "imageUrl must serialize as camelCase key")
	assert.Equal(t, true, decoded["is_official"], "is_official must serialize as snake_case key")
}
