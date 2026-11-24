// Package serverinstance holds the server's instance id: a random id made
// on the first start and kept in the database, so a client can tell this
// server from another one set up again from scratch at the same address. A
// restore from backup keeps it, and a fresh database gets a new one.
//
// It's the server's identity, not any one client's, so every surface that
// tells clients about the server reads it from here. Only the Shisho app's
// ServerInfo returns it so far.
package serverinstance

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/appsettings"
)

// settingsKey is the app_settings key holding the instance id. Nothing
// edits it: it's written once, by the first start to find none.
const settingsKey = "server_instance_id"

// Load returns the server's instance id, making and storing one when the
// database has none yet. Call it once at startup.
func Load(ctx context.Context, store *appsettings.Service) (string, error) {
	var id string
	found, err := store.GetJSON(ctx, settingsKey, &id)
	if err != nil {
		return "", errors.Wrap(err, "load the server instance id")
	}
	if found && id != "" {
		return id, nil
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", errors.WithStack(err)
	}
	id = hex.EncodeToString(random)
	// An empty id, from a database edited by hand, would match no app and
	// keep every one signing in again, so it's replaced.
	if found {
		if err := store.SetJSON(ctx, settingsKey, id); err != nil {
			return "", errors.Wrap(err, "store the server instance id")
		}
		return id, nil
	}
	// Another process starting on the same database at once keeps
	// whichever id was written first, so read it back.
	if err := store.InsertJSONIfAbsent(ctx, settingsKey, id); err != nil {
		return "", errors.Wrap(err, "store the server instance id")
	}
	if _, err := store.GetJSON(ctx, settingsKey, &id); err != nil {
		return "", errors.Wrap(err, "load the server instance id")
	}
	return id, nil
}
