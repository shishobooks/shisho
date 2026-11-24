package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// simctl names a group container by its path in the device the simulator
// was cloned from, so the folder is found again under the simulator's own
// home, where the clone has the same folders.
func TestLocalStorageFindsFilesOnMyIPhoneInTheSimulatorsOwnHome(t *testing.T) {
	t.Parallel()
	out := "group.com.apple.DocumentManager\t/Devices/BASE/data/Containers/Shared/AppGroup/C9F2\n" +
		"group.com.apple.FileProvider.LocalStorage\t/Devices/BASE/data/Containers/Shared/AppGroup/B218\n" +
		"group.com.apple.tipsnext\t/Devices/BASE/data/Containers/Shared/AppGroup/1C23\n"
	dir, ok := localStorage(out, "/Devices/CLONE/data")
	assert.True(t, ok)
	assert.Equal(t, "/Devices/CLONE/data/Containers/Shared/AppGroup/B218/File Provider Storage", dir)

	_, ok = localStorage("group.com.apple.DocumentManager\t/x/AppGroup/C9F2\n", "/Devices/CLONE/data")
	assert.False(t, ok)
}
