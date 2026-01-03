package main

import (
	"context"
	"net/http"

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

	srv, err := server.New(cfg, db)
	if err != nil {
		log.Err(err).Fatal("server error")
	}

	wrkr := worker.New(cfg, db)

	graceful := signals.Setup()

	go func() {
		log.Info("server started", logger.Data{"port": cfg.ServerPort})
		err := srv.ListenAndServe()
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
