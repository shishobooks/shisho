package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// filesFontName is the font the import flow picks in Files, a TTF from the
// web reader's own bundled fonts under a name of the harness's.
const filesFontName = "Harness Font.ttf"

// seedFiles puts the font the import flow picks into the simulator's Files,
// under On My iPhone, where the document picker's Browse tab opens. Flows
// can't add files themselves; Maestro adds only photos and videos.
func seedFiles(ctx context.Context, root, udid string) error {
	// Any installed app's listing names the system's groups; Files' local
	// storage is one of them.
	out, err := simctl(ctx, "get_app_container", udid, "com.apple.DocumentsApp", "groups")
	if err != nil {
		return err
	}
	dir, ok := localStorage(out)
	if !ok {
		return errors.New("the simulator has no Files storage on the device")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errors.WithStack(err)
	}
	font, err := os.ReadFile(filepath.Join(root, "app/assets/fonts/noto-serif/NotoSerif-Variable.ttf"))
	if err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(os.WriteFile(filepath.Join(dir, filesFontName), font, 0o600))
}

// localStorage finds the folder Files shows as On My iPhone in simctl's
// listing of an app's group containers.
func localStorage(groups string) (string, bool) {
	for _, line := range strings.Split(groups, "\n") {
		id, path, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if ok && id == "group.com.apple.FileProvider.LocalStorage" {
			return filepath.Join(path, "File Provider Storage"), true
		}
	}
	return "", false
}
