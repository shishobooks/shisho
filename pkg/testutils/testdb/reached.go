package testdb

import (
	"database/sql/driver"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/uptrace/bun/driver/sqliteshim"
	"modernc.org/sqlite"
)

// reachedFunc is the SQL function that the expressions Reached returns call.
const reachedFunc = "testdb_reached"

// probes maps a probe's id to the channel it closes.
var (
	probes   sync.Map
	probeSeq atomic.Int64
)

type probe struct {
	once    sync.Once
	reached chan struct{}
}

// The driver adds registered functions only to connections it opens later,
// and reads its registrations without a lock, so this runs in init, before
// any test opens a database.
func init() {
	drv, ok := sqliteshim.Driver().(*sqlite.Driver)
	if !ok {
		return // Reached fails the test that calls it.
	}
	err := drv.RegisterScalarFunction(reachedFunc, 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		id, _ := args[0].(int64)
		if p, ok := probes.Load(id); ok {
			p.(*probe).once.Do(func() { close(p.(*probe).reached) })
		}
		return int64(1), nil
	})
	if err != nil {
		panic(err)
	}
}

// Reached returns a SQL expression that evaluates to 1 and a channel that is
// closed the first time a running statement evaluates it. Put the expression
// in a statement to learn that the statement is executing, rather than
// sleeping and hoping it has started; canceling it then interrupts it
// mid-statement every time.
func Reached(t testing.TB) (expr string, reached <-chan struct{}) {
	t.Helper()
	if _, ok := sqliteshim.Driver().(*sqlite.Driver); !ok {
		t.Fatalf("testdb.Reached needs the modernc SQLite driver, got %T", sqliteshim.Driver())
	}
	id := probeSeq.Add(1)
	p := &probe{reached: make(chan struct{})}
	probes.Store(id, p)
	t.Cleanup(func() { probes.Delete(id) })
	return fmt.Sprintf("%s(%d)", reachedFunc, id), p.reached
}
