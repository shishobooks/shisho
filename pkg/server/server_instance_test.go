package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/shishoapp"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// The Shisho app tells servers apart by their instance id, so a restart
// keeps it and a fresh database gets another.
func TestNew_ServesOneInstanceIDAcrossRestarts(t *testing.T) {
	t.Parallel()
	instanceID := func(db *bun.DB) string {
		t.Helper()
		cfg := config.NewForTest()
		cfg.CacheDir = t.TempDir()
		srv, err := New(cfg, db, &worker.Worker{}, nil, nil, nil, nil, nil, nil, nil)
		require.NoError(t, err)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mobile/server", nil))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var info shishoapp.ServerInfo
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &info))
		return info.InstanceID
	}
	db := testdb.New(t)
	first := instanceID(db)
	assert.NotEmpty(t, first)
	assert.Equal(t, first, instanceID(db), "a restart keeps it")
	assert.NotEqual(t, first, instanceID(testdb.New(t)), "a fresh database gets another")
}
