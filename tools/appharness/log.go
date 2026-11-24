package main

import (
	"context"
	"io"
	"os"
)

type logKey struct{}

// withLog has the harness's messages for ctx's run go to w, which labels
// them by shard when shards run at once (parallel.go).
func withLog(ctx context.Context, w io.Writer) context.Context {
	return context.WithValue(ctx, logKey{}, w)
}

// logOut is where the harness's messages for ctx's run go: its shard's
// writer, or stderr.
func logOut(ctx context.Context) io.Writer {
	if w, ok := ctx.Value(logKey{}).(io.Writer); ok {
		return w
	}
	return os.Stderr
}
