package testdb

import (
	"context"
	"sync"
	"testing"

	"github.com/uptrace/bun"
)

// forwarders maps each database from open to the hook it was built with.
var forwarders sync.Map // *bun.DB -> *forwardHook

// AddQueryHook attaches hook to db, which must come from New or NewShared.
//
// Tests often add a hook after building the services under test, which hold
// db already. bun's WithQueryHook returns a copy those services never use, so
// every database from open carries a forwardHook from the start, and this adds
// hook to it in place.
func AddQueryHook(t testing.TB, db *bun.DB, hook bun.QueryHook) {
	t.Helper()
	fwd, ok := forwarders.Load(db)
	if !ok {
		t.Fatal("testdb.AddQueryHook: db did not come from testdb.New or testdb.NewShared")
	}
	fwd.(*forwardHook).add(hook)
}

// forwardHook passes each query to the hooks added to it, in the order bun
// runs its own: BeforeQuery in the order added, AfterQuery in reverse.
type forwardHook struct {
	mu    sync.RWMutex
	hooks []bun.QueryHook
}

func (f *forwardHook) add(hook bun.QueryHook) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hooks = append(f.hooks, hook)
}

func (f *forwardHook) snapshot() []bun.QueryHook {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.hooks[:len(f.hooks):len(f.hooks)]
}

func (f *forwardHook) BeforeQuery(ctx context.Context, event *bun.QueryEvent) context.Context {
	for _, hook := range f.snapshot() {
		ctx = hook.BeforeQuery(ctx, event)
	}
	return ctx
}

func (f *forwardHook) AfterQuery(ctx context.Context, event *bun.QueryEvent) {
	hooks := f.snapshot()
	for i := len(hooks) - 1; i >= 0; i-- {
		hooks[i].AfterQuery(ctx, event)
	}
}
