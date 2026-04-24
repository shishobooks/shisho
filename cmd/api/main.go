package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/robinjoseph08/golib/signals"
	"github.com/shishobooks/shisho/pkg/cbzpages"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/database"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/events"
	"github.com/shishobooks/shisho/pkg/logs"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/pdfpages"
	"github.com/shishobooks/shisho/pkg/plugins"
	"github.com/shishobooks/shisho/pkg/server"
	"github.com/shishobooks/shisho/pkg/version"
	"github.com/shishobooks/shisho/pkg/worker"
)

// shutdownHardDeadline is the wall-clock budget for the full graceful shutdown
// sequence (server + worker + db). If we exceed it, we os.Exit so the port is
// released promptly. Air's kill_delay defaults to 1s before SIGKILL, so the
// hard deadline is mainly insurance for `mise start:api` and other callers
// without a watchdog — if graceful takes longer than this, something is wrong
// and we'd rather drop the process than hold the port indefinitely.
const shutdownHardDeadline = 5 * time.Second

// serverShutdownTimeout bounds how long we wait for in-flight HTTP requests
// (including long-lived SSE streams) to finish before forcing connections
// closed. Shorter than shutdownHardDeadline so worker/db cleanup still runs.
const serverShutdownTimeout = 3 * time.Second

func main() {
	ctx := context.Background()

	broker := events.NewBroker()
	logBuffer := logs.NewRingBuffer(10_000, broker)
	logger.SetOutput(io.MultiWriter(logger.Output(), logBuffer))
	log := logger.New()

	log.Info("starting shisho", logger.Data{"version": version.Version})

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

	// Plugin system
	pluginService := plugins.NewService(db)
	pluginManager := plugins.NewManager(pluginService, cfg.PluginDir, cfg.PluginDataDir)
	if err := pluginManager.LoadAll(ctx); err != nil {
		log.Warn("plugin load errors occurred", logger.Data{"error": err.Error()})
	}

	dlCache := downloadcache.NewCache(filepath.Join(cfg.CacheDir, "downloads"), cfg.DownloadCacheMaxSizeBytes())
	cbzCache := cbzpages.NewCache(cfg.CacheDir)
	pdfCache := pdfpages.NewCache(cfg.CacheDir, cfg.PDFRenderDPI, cfg.PDFRenderQuality)

	wrkr := worker.New(cfg, db, pluginManager, broker, dlCache)

	srv, err := server.New(cfg, db, wrkr, pluginManager, broker, dlCache, cbzCache, pdfCache, logBuffer)
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

	// Watchdog: if any step below blocks past the hard deadline, force-exit
	// so the TCP listener is released. Without this, a stuck SSE stream, a
	// slow DB close, or a runaway job handler could hold port 3689 long
	// enough for the next air rebuild to race and hit "address already in
	// use". log.Fatal is not used here because it flushes buffered logs
	// which is itself a blocking operation.
	go func() {
		time.Sleep(shutdownHardDeadline)
		log.Error("graceful shutdown exceeded hard deadline, forcing exit", logger.Data{"deadline": shutdownHardDeadline.String()})
		os.Exit(1)
	}()

	// Tell SSE streams to exit their select loops now; otherwise each one
	// would hold an in-flight request until srv.Shutdown's timeout expires
	// or the client happened to disconnect. The broker's Done channel is
	// idempotent so this is safe to call before Shutdown.
	broker.Close()

	// Bound server shutdown so a hung SSE client or slow handler can't stall
	// the rest of the shutdown sequence. Shutdown returns ctx.DeadlineExceeded
	// on timeout; we then call Close to tear down remaining connections
	// forcefully.
	srvCtx, cancel := context.WithTimeout(ctx, serverShutdownTimeout)
	defer cancel()
	err = srv.Shutdown(srvCtx)
	if err != nil {
		log.Err(err).Error("server shutdown error")
		// Force-close any lingering connections (e.g. SSE streams that
		// didn't observe their request context cancellation in time).
		if closeErr := srv.Close(); closeErr != nil {
			log.Err(closeErr).Error("server force-close error")
		}
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
// Creates subdirectories: downloads (generated files), downloads/bulk (bulk zip files), cbz (extracted page images).
func initCacheDir(dir string) error {
	// Create subdirectories
	subdirs := []string{
		filepath.Join(dir, "downloads"),
		filepath.Join(dir, "downloads", "bulk"),
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
// Skips silently if tmp/ directory doesn't exist (e.g., in Docker).
func writePortFile(port int) error {
	if _, err := os.Stat("tmp"); os.IsNotExist(err) {
		return nil
	}
	return os.WriteFile("tmp/api.port", []byte(strconv.Itoa(port)), 0600)
}
