package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessEnv_AFlowsAppProtocolsComeLastSoTheyWin(t *testing.T) {
	t.Parallel()
	s := &server{env: []string{"SERVER_PORT=1234"}}
	env := s.processEnv()
	assert.Contains(t, env, "SERVER_PORT=1234")
	assert.NotContains(t, env, "SHISHO_DEV_APP_PROTOCOLS=2-2")

	s.appProtocols = "2-2"
	env = s.processEnv()
	assert.Equal(t, "SHISHO_DEV_APP_PROTOCOLS=2-2", env[len(env)-1], "a later entry overrides one the shell set")
}
