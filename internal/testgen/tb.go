package testgen

import "context"

// TB is the part of testing.TB the file generators use, so a tool outside a
// test, such as the Shisho app's harness, can make the same fixtures.
type TB interface {
	Helper()
	Fatalf(format string, args ...any)
	Context() context.Context
}
