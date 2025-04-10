package database

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

type key int

const ctxKey key = 0

func WithLogging(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKey, true)
}

type logQueryHook struct {
	log logger.Logger
}

func (*logQueryHook) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (qh *logQueryHook) AfterQuery(ctx context.Context, event *bun.QueryEvent) {
	enabled, ok := ctx.Value(ctxKey).(bool)
	if !ok || !enabled {
		return
	}

	qh.log.Debug(event.Query)
}

func New(cfg *config.Config) (*bun.DB, error) {
	sqldb, err := sql.Open(sqliteshim.ShimName, cfg.DatabaseFilePath)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())

	// print out all queries in debug mode
	if cfg.DatabaseDebug {
		db.AddQueryHook(&logQueryHook{logger.NewWithLevel("debug")})
	}

	// Retry up to a few times to ensure that the database can connect.
	for i := 0; i < cfg.DatabaseConnectRetryCount; i++ {
		_, err = db.Exec("SELECT 1")
		if err != nil {
			time.Sleep(cfg.DatabaseConnectRetryDelay)
			continue
		}
		// We've successfully connected.
		break
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return db, nil
}
