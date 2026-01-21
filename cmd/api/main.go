package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/robinjoseph08/golib/signals"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/database"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/server"
	"github.com/shishobooks/shisho/pkg/worker"
)

func main() {
	ctx := context.Background()
	log := logger.New()

	cfg, err := config.New()
	if err != nil {
		log.Err(err).Fatal("config error")
	}

	// Initialize cache directories
	if err := initCacheDir(cfg.CacheDir); err != nil {
		log.Err(err).Fatal("cache directory error")
	}
	log.Info("cache directory initialized", logger.Data{"path": cfg.CacheDir})

	db, err := database.New(cfg)
	if err != nil {
		log.Err(err).Fatal("database error")
	}

	// Check that FTS5 is available before running migrations
	err = database.CheckFTS5Support(db)
	if err != nil {
		log.Err(err).Fatal("FTS5 check failed")
	}

	group, err := migrations.BringUpToDate(ctx, db)
	if err != nil {
		log.Err(err).Fatal("migrations error")
	}
	if group.ID == 0 {
		log.Info("no new migrations to run")
	} else {
		log.Info("migrated to new group", logger.Data{"group_id": group.ID, "migration_names": group.Migrations.String()})
	}

	wrkr := worker.New(cfg, db)

	srv, err := server.New(cfg, db, wrkr)
	if err != nil {
		log.Err(err).Fatal("server error")
	}

	graceful := signals.Setup()

	go func() {
		addr := fmt.Sprintf(":%d", cfg.ServerPort)
		lc := net.ListenConfig{}
		listener, err := lc.Listen(ctx, "tcp", addr)
		if err != nil {
			log.Err(err).Fatal("failed to bind port")
		}

		// Extract actual port (useful when ServerPort is 0)
		actualPort := listener.Addr().(*net.TCPAddr).Port
		log.Info("server started", logger.Data{"port": actualPort})

		// Write port file for Vite to read
		if err := writePortFile(actualPort); err != nil {
			log.Err(err).Error("failed to write port file")
		}

		err = srv.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Err(err).Fatal("server stopped")
		}
		log.Info("server stopped")
	}()

	wrkr.Start()
	log.Info("worker started")

	<-graceful
	log.Info("starting graceful shutdown")

	err = srv.Shutdown(ctx)
	if err != nil {
		log.Err(err).Error("server shutdown error")
	}
	log.Info("server shutdown")

	wrkr.Shutdown()
	log.Info("worker shutdown")

	err = db.Close()
	if err != nil {
		log.Err(err).Error("database close error")
	}
	log.Info("database closed")
}

// initCacheDir creates the cache directories and verifies write permissions.
// Creates subdirectories: downloads (generated files), cbz (extracted page images).
func initCacheDir(dir string) error {
	// Create subdirectories
	subdirs := []string{
		filepath.Join(dir, "downloads"),
		filepath.Join(dir, "cbz"),
	}

	for _, subdir := range subdirs {
		if err := os.MkdirAll(subdir, 0755); err != nil {
			return errors.Wrapf(err, "failed to create cache directory: %s", subdir)
		}
	}

	// Verify write permissions by creating and removing a temp file
	testFile := filepath.Join(dir, ".write_test")
	f, err := os.Create(testFile)
	if err != nil {
		return errors.Wrapf(err, "cache directory is not writable: %s", dir)
	}
	f.Close()

	if err := os.Remove(testFile); err != nil {
		return errors.Wrapf(err, "failed to clean up write test file: %s", testFile)
	}

	return nil
}

// writePortFile writes the server's actual port to tmp/api.port for frontend dev server.
func writePortFile(port int) error {
	return os.WriteFile("tmp/api.port", []byte(strconv.Itoa(port)), 0600)
}
