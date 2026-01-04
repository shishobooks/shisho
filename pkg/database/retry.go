package database

import (
	"context"
	"database/sql/driver"
	"math/rand"
	"strings"
	"time"
)

// driverConnector wraps a driver.Driver to implement driver.Connector.
// This allows using sql.OpenDB() with drivers that don't natively support OpenConnector.
type driverConnector struct {
	driver driver.Driver
	dsn    string
}

// newDriverConnector creates a connector from a driver and DSN.
func newDriverConnector(drv driver.Driver, dsn string) *driverConnector {
	return &driverConnector{driver: drv, dsn: dsn}
}

func (dc *driverConnector) Connect(_ context.Context) (driver.Conn, error) {
	return dc.driver.Open(dc.dsn)
}

func (dc *driverConnector) Driver() driver.Driver {
	return dc.driver
}

// retryConnector wraps a driver.Connector and adds retry logic for SQLITE_BUSY errors.
type retryConnector struct {
	connector  driver.Connector
	maxRetries int
}

// newRetryConnector creates a new retryConnector that wraps the given connector.
func newRetryConnector(connector driver.Connector, maxRetries int) *retryConnector {
	return &retryConnector{
		connector:  connector,
		maxRetries: maxRetries,
	}
}

func (rc *retryConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := rc.connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &retryConn{conn: conn, maxRetries: rc.maxRetries}, nil
}

func (rc *retryConnector) Driver() driver.Driver {
	return rc.connector.Driver()
}

// retryConn wraps a driver.Conn and adds retry logic for SQLITE_BUSY errors.
type retryConn struct {
	conn       driver.Conn
	maxRetries int
}

// isBusyError checks if the error is a SQLite BUSY or LOCKED error.
// Works with both mattn/go-sqlite3 and modernc.org/sqlite drivers.
func isBusyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Check for common SQLite busy/locked error patterns
	return strings.Contains(errStr, "database is locked") ||
		strings.Contains(errStr, "database table is locked") ||
		strings.Contains(errStr, "SQLITE_BUSY") ||
		strings.Contains(errStr, "SQLITE_LOCKED") ||
		strings.Contains(errStr, "(5)") || // SQLITE_BUSY error code
		strings.Contains(errStr, "(6)") // SQLITE_LOCKED error code
}

// retryWithBackoff executes a function with exponential backoff on SQLITE_BUSY errors.
func retryWithBackoff(ctx context.Context, maxRetries int, fn func() error) error {
	var err error
	baseDelay := 50 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}

		if !isBusyError(err) {
			return err
		}

		if attempt == maxRetries {
			return err
		}

		// Calculate delay with exponential backoff and jitter
		delay := baseDelay * time.Duration(1<<attempt)
		// Add jitter (up to 25% of delay)
		jitter := time.Duration(rand.Int63n(int64(delay / 4)))
		delay += jitter

		// Cap delay at 2 seconds
		if delay > 2*time.Second {
			delay = 2 * time.Second
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Continue to next retry
		}
	}

	return err
}

// Prepare implements driver.Conn.
func (c *retryConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &retryStmt{stmt: stmt, maxRetries: c.maxRetries}, nil
}

// Close implements driver.Conn.
func (c *retryConn) Close() error {
	return c.conn.Close()
}

// Begin implements driver.Conn.
func (c *retryConn) Begin() (driver.Tx, error) {
	var tx driver.Tx
	err := retryWithBackoff(context.Background(), c.maxRetries, func() error {
		var innerErr error
		tx, innerErr = c.conn.Begin() //nolint:staticcheck // deprecated but required for interface
		return innerErr
	})
	return tx, err
}

// BeginTx implements driver.ConnBeginTx.
func (c *retryConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if connBeginTx, ok := c.conn.(driver.ConnBeginTx); ok {
		var tx driver.Tx
		err := retryWithBackoff(ctx, c.maxRetries, func() error {
			var innerErr error
			tx, innerErr = connBeginTx.BeginTx(ctx, opts)
			return innerErr
		})
		return tx, err
	}
	return c.Begin()
}

// PrepareContext implements driver.ConnPrepareContext.
func (c *retryConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if connPrepareContext, ok := c.conn.(driver.ConnPrepareContext); ok {
		stmt, err := connPrepareContext.PrepareContext(ctx, query)
		if err != nil {
			return nil, err
		}
		return &retryStmt{stmt: stmt, maxRetries: c.maxRetries}, nil
	}
	return c.Prepare(query)
}

// ExecContext implements driver.ExecerContext.
func (c *retryConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if execerContext, ok := c.conn.(driver.ExecerContext); ok {
		var result driver.Result
		err := retryWithBackoff(ctx, c.maxRetries, func() error {
			var innerErr error
			result, innerErr = execerContext.ExecContext(ctx, query, args)
			return innerErr
		})
		return result, err
	}
	return nil, driver.ErrSkip
}

// QueryContext implements driver.QueryerContext.
func (c *retryConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if queryerContext, ok := c.conn.(driver.QueryerContext); ok {
		var rows driver.Rows
		err := retryWithBackoff(ctx, c.maxRetries, func() error {
			var innerErr error
			rows, innerErr = queryerContext.QueryContext(ctx, query, args)
			return innerErr
		})
		return rows, err
	}
	return nil, driver.ErrSkip
}

// Ping implements driver.Pinger.
func (c *retryConn) Ping(ctx context.Context) error {
	if pinger, ok := c.conn.(driver.Pinger); ok {
		return pinger.Ping(ctx)
	}
	return nil
}

// ResetSession implements driver.SessionResetter.
func (c *retryConn) ResetSession(ctx context.Context) error {
	if resetter, ok := c.conn.(driver.SessionResetter); ok {
		return resetter.ResetSession(ctx)
	}
	return nil
}

// IsValid implements driver.Validator.
func (c *retryConn) IsValid() bool {
	if validator, ok := c.conn.(driver.Validator); ok {
		return validator.IsValid()
	}
	return true
}

// retryStmt wraps a driver.Stmt and adds retry logic.
type retryStmt struct {
	stmt       driver.Stmt
	maxRetries int
}

// Close implements driver.Stmt.
func (s *retryStmt) Close() error {
	return s.stmt.Close()
}

// NumInput implements driver.Stmt.
func (s *retryStmt) NumInput() int {
	return s.stmt.NumInput()
}

// Exec implements driver.Stmt.
func (s *retryStmt) Exec(args []driver.Value) (driver.Result, error) {
	var result driver.Result
	err := retryWithBackoff(context.Background(), s.maxRetries, func() error {
		var innerErr error
		result, innerErr = s.stmt.Exec(args) //nolint:staticcheck // deprecated but required for interface
		return innerErr
	})
	return result, err
}

// Query implements driver.Stmt.
func (s *retryStmt) Query(args []driver.Value) (driver.Rows, error) {
	var rows driver.Rows
	err := retryWithBackoff(context.Background(), s.maxRetries, func() error {
		var innerErr error
		rows, innerErr = s.stmt.Query(args) //nolint:staticcheck // deprecated but required for interface
		return innerErr
	})
	return rows, err
}

// ExecContext implements driver.StmtExecContext.
func (s *retryStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if stmtExecContext, ok := s.stmt.(driver.StmtExecContext); ok {
		var result driver.Result
		err := retryWithBackoff(ctx, s.maxRetries, func() error {
			var innerErr error
			result, innerErr = stmtExecContext.ExecContext(ctx, args)
			return innerErr
		})
		return result, err
	}

	// Fallback to non-context version
	values := make([]driver.Value, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}
	return s.Exec(values)
}

// QueryContext implements driver.StmtQueryContext.
func (s *retryStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if stmtQueryContext, ok := s.stmt.(driver.StmtQueryContext); ok {
		var rows driver.Rows
		err := retryWithBackoff(ctx, s.maxRetries, func() error {
			var innerErr error
			rows, innerErr = stmtQueryContext.QueryContext(ctx, args)
			return innerErr
		})
		return rows, err
	}

	// Fallback to non-context version
	values := make([]driver.Value, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}
	return s.Query(values)
}
