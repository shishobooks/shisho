// Package sqliteconn opens SQLite connections that run a fixed set of
// statements, usually pragmas, on every connection the pool opens. It has no
// Shisho imports, so both pkg/database and pkg/testutils/testdb can use it.
//
// Pragmas such as foreign_keys and busy_timeout apply only to the connection
// that ran them. database/sql discards a connection whose query is canceled
// mid-statement and opens a replacement with SQLite's defaults, so running
// them once at startup is not enough.
package sqliteconn

import (
	"context"
	"database/sql/driver"

	"github.com/pkg/errors"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// NewConnector returns a connector for dsn on the sqliteshim driver that runs
// statements, in order, on each new connection before handing it out.
func NewConnector(dsn string, statements ...string) (driver.Connector, error) {
	drv := sqliteshim.Driver()

	var connector driver.Connector
	if drvCtx, ok := drv.(driver.DriverContext); ok {
		var err error
		connector, err = drvCtx.OpenConnector(dsn)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	} else {
		connector = &driverConnector{driver: drv, dsn: dsn}
	}

	return &initConnector{connector: connector, statements: statements}, nil
}

// driverConnector adapts a driver without OpenConnector to driver.Connector.
type driverConnector struct {
	driver driver.Driver
	dsn    string
}

func (dc *driverConnector) Connect(_ context.Context) (driver.Conn, error) {
	return dc.driver.Open(dc.dsn)
}

func (dc *driverConnector) Driver() driver.Driver {
	return dc.driver
}

// initConnector runs statements on every connection it opens.
type initConnector struct {
	connector  driver.Connector
	statements []string
}

func (ic *initConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := ic.connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	for _, statement := range ic.statements {
		if err := exec(ctx, conn, statement); err != nil {
			_ = conn.Close()
			return nil, errors.Wrapf(err, "failed to run %q on new connection", statement)
		}
	}
	return conn, nil
}

func (ic *initConnector) Driver() driver.Driver {
	return ic.connector.Driver()
}

func exec(ctx context.Context, conn driver.Conn, statement string) error {
	if execer, ok := conn.(driver.ExecerContext); ok {
		_, err := execer.ExecContext(ctx, statement, nil)
		if !errors.Is(err, driver.ErrSkip) {
			return err
		}
	}
	stmt, err := conn.Prepare(statement)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(nil) //nolint:staticcheck // the fallback for drivers without ExecerContext
	return err
}
