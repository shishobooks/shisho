package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/internal/devtool"
)

// qaPort is where a QA server looks for a port first. It takes the first
// free one from here, as `mise start` does, so a phone usually keeps the same
// address between runs.
const qaPort = 3789

// runServe starts the harness's seeded server and keeps it running until
// interrupted, printing what to sign in with. Its files go when it stops.
// `mise start:qa` builds the web app first, so the server serves it too.
func runServe(ctx context.Context) error {
	env, err := devtool.ResolveEnvironment(ctx)
	if err != nil {
		return errors.Wrap(err, "resolve the worktree")
	}
	lease, err := (&devtool.FilePortAllocator{LockRoot: env.PortLockRoot()}).Acquire(qaPort)
	if err != nil {
		return errors.Wrap(err, "reserve a port")
	}
	defer func() { _ = lease.Close() }()

	dir, err := os.MkdirTemp("", "shisho-qa-server-")
	if err != nil {
		return errors.WithStack(err)
	}
	defer os.RemoveAll(dir)

	library := filepath.Join(dir, "library")
	if err := writeFixtures(ctx, library); err != nil {
		return errors.Wrap(err, "make fixtures")
	}
	// Every interface, so a phone on the same network can sign in too. Its
	// session cookie is apart from `mise start`'s, which shares localhost.
	srv, err := startServer(ctx, env.CurrentRoot, dir, "0.0.0.0", lease.Port(),
		"SHISHO_COOKIE_NAMESPACE="+qaCookieNamespace(env))
	if err != nil {
		return err
	}
	defer srv.Stop()
	// The admin listens on the whole network, so its password is new each
	// run rather than one anyone can read in the repository.
	password, err := randomPassword()
	if err != nil {
		return err
	}
	if err := srv.seed(ctx, library, password); err != nil {
		return err
	}
	fmt.Printf("Server:   http://localhost:%s\n", srv.port)
	if lan := devtool.LANAddress(); lan != "" {
		fmt.Printf("LAN:      http://%s:%s (a phone on this network)\n", lan, srv.port)
	}
	fmt.Printf("Username: %s\nPassword: %s\nLog:      %s\nCtrl-C stops it and deletes its data.\n",
		harnessUsername, password, filepath.Join(dir, "server.log"))
	<-ctx.Done()
	return nil
}

func randomPassword() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", errors.WithStack(err)
	}
	return hex.EncodeToString(b), nil
}

// qaCookieNamespace names the QA server's session cookie: the worktree's
// own namespace, marked as QA's.
func qaCookieNamespace(env devtool.Environment) string {
	if namespace := env.CookieNamespace(); namespace != "" {
		return namespace + "_qa"
	}
	return "qa"
}
