package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/internal/devtool"
)

// runServe starts the harness's seeded server and keeps it running until
// interrupted, printing what to sign in with. Its files go when it stops.
func runServe(ctx context.Context) error {
	root, err := os.Getwd()
	if err != nil {
		return errors.WithStack(err)
	}
	dir, err := os.MkdirTemp("", "shisho-app-server-")
	if err != nil {
		return errors.WithStack(err)
	}
	defer os.RemoveAll(dir)

	library := filepath.Join(dir, "library")
	if err := writeFixtures(ctx, library); err != nil {
		return errors.Wrap(err, "make fixtures")
	}
	// Every interface, so a phone on the same network can sign in too.
	srv, err := startServer(ctx, root, dir, "0.0.0.0")
	if err != nil {
		return err
	}
	defer srv.Stop()
	if err := srv.seed(ctx, library); err != nil {
		return err
	}
	fmt.Printf("Server:   %s (simulator)\n", srv.URL)
	if lan := devtool.LANAddress(); lan != "" {
		fmt.Printf("LAN:      http://%s:%s (a phone on this network)\n", lan, srv.port)
	}
	fmt.Printf("Username: %s\nPassword: %s\nCtrl-C stops it and deletes its data.\n", harnessUsername, harnessPassword)
	<-ctx.Done()
	return nil
}
