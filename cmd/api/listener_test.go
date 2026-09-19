package main

import (
	"net"
	"testing"

	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/server"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListenServer_UsesConfiguredHost(t *testing.T) {
	t.Parallel()
	cfg := config.NewForTest()
	cfg.Environment = ""
	cfg.ServerHost = "127.0.0.1"
	cfg.ServerPort = 0
	srv, err := server.New(cfg, nil, &worker.Worker{}, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)

	listener, err := listenServer(t.Context(), srv)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, listener.Close()) })
	addr := listener.Addr().(*net.TCPAddr)
	assert.Equal(t, "127.0.0.1", addr.IP.String())
	assert.Positive(t, addr.Port)
}
