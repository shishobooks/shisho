package plugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetEventCallback_ReceivesEvents(t *testing.T) {
	pluginDir := t.TempDir()
	db := setupTestDB(t)
	service := NewService(db)
	manager := NewManager(service, pluginDir, "")

	var received []PluginEvent
	manager.SetEventCallback(func(event PluginEvent) {
		received = append(received, event)
	})

	// Emit some events
	manager.emitEvent(PluginEventInstalled, "test", "my-plugin", []string{"fileParser"})
	manager.emitEvent(PluginEventDisabled, "test", "my-plugin", nil)
	manager.emitEvent(PluginEventUninstalled, "test", "my-plugin", nil)

	require.Len(t, received, 3)

	assert.Equal(t, PluginEventInstalled, received[0].Type)
	assert.Equal(t, "test", received[0].Scope)
	assert.Equal(t, "my-plugin", received[0].ID)
	assert.Equal(t, []string{"fileParser"}, received[0].Hooks)

	assert.Equal(t, PluginEventDisabled, received[1].Type)
	assert.Nil(t, received[1].Hooks)

	assert.Equal(t, PluginEventUninstalled, received[2].Type)
}

func TestEmitEvent_NoCallback_NoPanic(t *testing.T) {
	pluginDir := t.TempDir()
	db := setupTestDB(t)
	service := NewService(db)
	manager := NewManager(service, pluginDir, "")

	// Should not panic when no callback is set
	assert.NotPanics(t, func() {
		manager.emitEvent(PluginEventInstalled, "test", "plugin", nil)
	})
}

func TestSetEventCallback_ReplacesExisting(t *testing.T) {
	pluginDir := t.TempDir()
	db := setupTestDB(t)
	service := NewService(db)
	manager := NewManager(service, pluginDir, "")

	var first, second []PluginEvent
	manager.SetEventCallback(func(event PluginEvent) {
		first = append(first, event)
	})
	manager.SetEventCallback(func(event PluginEvent) {
		second = append(second, event)
	})

	manager.emitEvent(PluginEventEnabled, "test", "plugin", []string{"metadataEnricher"})

	assert.Empty(t, first, "first callback should not receive events after replacement")
	require.Len(t, second, 1)
	assert.Equal(t, PluginEventEnabled, second[0].Type)
}
