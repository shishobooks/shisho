package plugins

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConfigGetter implements ConfigGetter for testing.
type mockConfigGetter struct {
	configs map[string]*string
}

func (m *mockConfigGetter) GetConfigRaw(_ context.Context, _, _, key string) (*string, error) {
	v := m.configs[key]
	return v, nil
}

func (m *mockConfigGetter) GetAllConfigRaw(_ context.Context, _, _ string) (map[string]*string, error) {
	return m.configs, nil
}

// newTestRuntime creates a minimal Runtime with a fresh goja VM for testing.
func newTestRuntime(scope, pluginID string) *Runtime {
	return &Runtime{
		vm:       goja.New(),
		scope:    scope,
		pluginID: pluginID,
		manifest: &Manifest{
			ManifestVersion: 1,
			ID:              pluginID,
			Name:            "Test Plugin",
			Version:         "1.0.0",
			Capabilities:    Capabilities{},
		},
	}
}

func strPtr(s string) *string {
	return &s
}

func TestInjectHostAPIs_LogNamespace(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}

	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	// Verify calling each log level doesn't panic
	_, err = rt.vm.RunString(`shisho.log.debug("debug message")`)
	require.NoError(t, err)

	_, err = rt.vm.RunString(`shisho.log.info("info message")`)
	require.NoError(t, err)

	_, err = rt.vm.RunString(`shisho.log.warn("warn message")`)
	require.NoError(t, err)

	_, err = rt.vm.RunString(`shisho.log.error("error message")`)
	require.NoError(t, err)
}

func TestInjectHostAPIs_ConfigGet(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{
		"api_key":  strPtr("secret123"),
		"base_url": strPtr("https://example.com"),
	}}

	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`shisho.config.get("api_key")`)
	require.NoError(t, err)
	assert.Equal(t, "secret123", val.String())

	val, err = rt.vm.RunString(`shisho.config.get("base_url")`)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", val.String())
}

func TestInjectHostAPIs_ConfigGet_Undefined(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}

	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`shisho.config.get("nonexistent")`)
	require.NoError(t, err)
	assert.True(t, goja.IsUndefined(val), "expected undefined for missing config key")
}

func TestInjectHostAPIs_ConfigGetAll(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{
		"key1": strPtr("value1"),
		"key2": strPtr("value2"),
		"key3": nil, // nil values should not appear in result
	}}

	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`JSON.stringify(shisho.config.getAll())`)
	require.NoError(t, err)

	// Parse the JSON and verify keys
	result := val.String()
	assert.Contains(t, result, `"key1":"value1"`)
	assert.Contains(t, result, `"key2":"value2"`)
	assert.NotContains(t, result, `"key3"`)
}

func TestInjectHostAPIs_FFmpegRequiresCapability(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}

	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	// Without ffmpegAccess capability, run should throw an error
	_, err = rt.vm.RunString(`shisho.ffmpeg.run(["-version"])`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin does not declare ffmpegAccess capability")
}

func TestInjectHostAPIs_LogPluginTag(t *testing.T) {
	// Verify the runtime uses the correct scope/pluginID combination
	rt := newTestRuntime("community", "my-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}

	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	// The log calls should not panic (they log internally with the plugin tag)
	_, err = rt.vm.RunString(`shisho.log.info("test with tag")`)
	assert.NoError(t, err)
}

func TestInjectHostAPIs_ConfigGetAll_Empty(t *testing.T) {
	rt := newTestRuntime("official", "test-plugin")
	cfg := &mockConfigGetter{configs: map[string]*string{}}

	err := InjectHostAPIs(rt, cfg)
	require.NoError(t, err)

	val, err := rt.vm.RunString(`JSON.stringify(shisho.config.getAll())`)
	require.NoError(t, err)
	assert.Equal(t, "{}", val.String())
}
