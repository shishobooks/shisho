package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllDisabled(t *testing.T) {
	t.Parallel()
	printed := "\n\tdisabled services = {\n" +
		"\t\t\"com.apple.apsd\" => disabled\n" +
		"\t\t\"com.apple.nanonewscd\" => enabled\n" +
		"\t\t\"com.apple.mobileassetd\" => enabled\n" +
		"\t}\n"
	services := []string{"com.apple.apsd", "com.apple.mobileassetd"}

	assert.False(t, allDisabled(printed, services), "mobileassetd is enabled")
	assert.False(t, allDisabled("\n\tdisabled services = {\n\t\t\"com.apple.apsd\" => disabled\n\t}\n", services), "mobileassetd isn't listed")
	assert.True(t, allDisabled(printed, services[:1]))
	assert.True(t, allDisabled("\t\t\"com.apple.apsd\" => disabled\n\t\t\"com.apple.mobileassetd\" => disabled\n", services))
	assert.False(t, allDisabled("\t\t\"com.apple.apsd.helper\" => disabled\n", services[:1]), "another service")
}

func TestAnyRunning(t *testing.T) {
	t.Parallel()
	listed := "PID\tStatus\tLabel\n" +
		"-\t0\tcom.apple.MobileAsset.ManifestStorageService\n" +
		"18254\t0\tcom.apple.nsurlsessiond\n" +
		"-\t0\tcom.apple.apsd\n"
	services := []string{"com.apple.apsd", "com.apple.mobileassetd"}

	assert.False(t, anyRunning(listed, services), "apsd is loaded but not running, mobileassetd is gone")
	assert.True(t, anyRunning(listed+"18660\t0\tcom.apple.mobileassetd\n", services))
	assert.False(t, anyRunning("18660\t0\tcom.apple.mobileassetd.helper\n", services), "another service")
}

func TestUnusedBases(t *testing.T) {
	t.Parallel()
	devices := []simctlDevice{
		{UDID: "current", Name: baseSimulatorName},
		{UDID: "old-unused", Name: "Shisho Harness Base 3"},
		{UDID: "old-used", Name: "Shisho Harness Base 4"},
		{UDID: "newer-used", Name: baseSimulatorPrefix + "99"},
		{UDID: "newer-unused", Name: baseSimulatorPrefix + "98"},
		{UDID: "worktree", Name: "Shisho Worktree flows abc"},
		{UDID: "someone-elses", Name: "Shisho iOS 26.5 base"},
		{UDID: "hand-made", Name: baseSimulatorPrefix + "QA"},
	}
	used := map[string]bool{"old-used": true, "newer-used": true, "worktree": true}

	assert.ElementsMatch(t, []string{"old-unused", "newer-unused"}, unusedBases(devices, used))
}

func TestBaseNameIsOutsideTheOldPrefix(t *testing.T) {
	t.Parallel()
	// A harness from before this name deletes every base under its prefix
	// but its own, so a base under it would be deleted by each run in a
	// worktree on an older commit.
	assert.NotContains(t, baseSimulatorName, oldBaseSimulatorPrefix)
	assert.True(t, strings.HasPrefix(baseSimulatorName, baseSimulatorPrefix))
}

// fakeLaunchd stands in for simctl on a booting simulator: launchctl's
// commands fail until the simulator's user domain is ready, and
// `bootstatus` returns once boot is closed.
type fakeLaunchd struct {
	mu       sync.Mutex
	ready    bool
	disabled map[string]bool
	running  map[string]bool
	// readyAtBoot makes the domain ready only when bootstatus returns, as on
	// a slow CI runner.
	readyAtBoot bool
	// neverReady keeps launchctl failing, even after the boot.
	neverReady bool
	boot       chan struct{}
	bootErr    error
	// bootOnRefusal finishes the boot once launchctl first refuses a
	// command, so the quieting during the boot is sure to have failed.
	bootOnRefusal bool
	refused       bool
}

func newFakeLaunchd() *fakeLaunchd {
	return &fakeLaunchd{
		disabled: map[string]bool{},
		running:  map[string]bool{"com.apple.apsd": true, "com.apple.mobileassetd": true},
		boot:     make(chan struct{}),
	}
}

func (f *fakeLaunchd) simctl(ctx context.Context, args ...string) (string, error) {
	if args[0] == "bootstatus" {
		select {
		case <-f.boot:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		f.mu.Lock()
		f.ready = f.ready || (f.readyAtBoot && !f.neverReady)
		f.mu.Unlock()
		return "", f.bootErr
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.ready || f.neverReady {
		if f.bootOnRefusal && !f.refused {
			close(f.boot)
		}
		f.refused = true
		return "", errors.New("launchctl: no user domain yet")
	}
	launchctl := args[3:]
	switch launchctl[0] {
	case "print-disabled":
		var out strings.Builder
		out.WriteString("\tdisabled services = {\n")
		for label := range f.disabled {
			fmt.Fprintf(&out, "\t\t%q => disabled\n", label)
		}
		out.WriteString("\t}\n")
		return out.String(), nil
	case "list":
		var out strings.Builder
		out.WriteString("PID\tStatus\tLabel\n")
		for label := range f.running {
			fmt.Fprintf(&out, "123\t0\t%s\n", label)
		}
		return out.String(), nil
	case "disable":
		f.disabled[strings.TrimPrefix(launchctl[1], "system/")] = true
	case "bootout":
		delete(f.running, strings.TrimPrefix(launchctl[1], "system/"))
	}
	return "", nil
}

func (f *fakeLaunchd) quiet() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.running) == 0 && f.disabled["com.apple.apsd"] && f.disabled["com.apple.mobileassetd"]
}

func TestBootQuietlyQuietsWhileTheSimulatorBoots(t *testing.T) {
	t.Parallel()
	f := newFakeLaunchd()
	f.ready = true
	go func() {
		// The boot finishes only once the services are quiet, so they were
		// quieted during it.
		for deadline := time.Now().Add(10 * time.Second); !f.quiet() && time.Now().Before(deadline); {
			time.Sleep(10 * time.Millisecond)
		}
		close(f.boot)
	}()

	require.NoError(t, bootQuietly(context.Background(), f.simctl, "udid", time.Millisecond))
	assert.True(t, f.quiet())
}

func TestBootQuietlyQuietsAfterTheBootWhenLaunchdWasntReady(t *testing.T) {
	t.Parallel()
	f := newFakeLaunchd()
	f.readyAtBoot = true
	f.bootOnRefusal = true

	require.NoError(t, bootQuietly(context.Background(), f.simctl, "udid", 5*time.Second))
	assert.True(t, f.quiet())
}

func TestBootQuietlyStopsAServiceDisabledButStillRunning(t *testing.T) {
	t.Parallel()
	// As a try cut off between the disable and the bootout leaves it.
	f := newFakeLaunchd()
	f.ready = true
	f.disabled = map[string]bool{"com.apple.apsd": true, "com.apple.mobileassetd": true}
	f.running = map[string]bool{"com.apple.mobileassetd": true}
	close(f.boot)

	require.NoError(t, bootQuietly(context.Background(), f.simctl, "udid", 5*time.Second))
	assert.True(t, f.quiet())
}

func TestBootQuietlyGivesUpOnQuietingButNotTheBoot(t *testing.T) {
	t.Parallel()
	f := newFakeLaunchd()
	f.neverReady = true
	close(f.boot)

	require.NoError(t, bootQuietly(context.Background(), f.simctl, "udid", 100*time.Millisecond))
	assert.False(t, f.quiet())
}

func TestBootQuietlyReturnsTheBootsError(t *testing.T) {
	t.Parallel()
	f := newFakeLaunchd()
	f.neverReady = true
	f.bootErr = errors.New("boot failed")
	close(f.boot)

	assert.ErrorIs(t, bootQuietly(context.Background(), f.simctl, "udid", time.Minute), f.bootErr)
}

func TestWorktreeSimulatorBases(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, writeSimulatorState(filepath.Join(dir, "flows-a.json"), worktreeSimulatorState{UDID: "sim-a", Base: "base-4"}))
	require.NoError(t, writeSimulatorState(filepath.Join(dir, "tests-b.json"), worktreeSimulatorState{UDID: "sim-b", Base: "base-5"}))

	bases, err := worktreeSimulatorBases(dir)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"base-4": true, "base-5": true}, bases)

	// A state file that can't be read might name any base, so none is
	// known to be unused.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "flows-c.json"), []byte(`{"udid":`), 0o600))
	_, err = worktreeSimulatorBases(dir)
	assert.Error(t, err)
}
