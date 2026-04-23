package kobo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildInitResources_OverridesImageKeys(t *testing.T) {
	t.Parallel()

	base := "https://example.com/kobo/key/library/1"
	res, err := buildInitResources(base)
	require.NoError(t, err)

	assert.Equal(t, base, res["image_host"], "image_host must point at our server")
	assert.Equal(t,
		base+"/v1/books/{ImageId}/thumbnail/{Width}/{Height}/false/image.jpg",
		res["image_url_template"],
	)
	assert.Equal(t,
		base+"/v1/books/{ImageId}/thumbnail/{Width}/{Height}/{Quality}/{IsGreyscale}/image.jpg",
		res["image_url_quality_template"],
	)

	// A handful of native keys must survive the override.
	assert.Contains(t, res, "library_sync")
	assert.Contains(t, res, "user_profile")
	assert.Contains(t, res, "deals")
}

func TestBuildInitResources_DoesNotMutateGlobal(t *testing.T) {
	t.Parallel()

	res1, err := buildInitResources("https://a.example/base")
	require.NoError(t, err)
	res2, err := buildInitResources("https://b.example/base")
	require.NoError(t, err)

	assert.Equal(t, "https://a.example/base", res1["image_host"])
	assert.Equal(t, "https://b.example/base", res2["image_host"])
}
