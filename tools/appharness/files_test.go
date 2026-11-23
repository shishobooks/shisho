package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalStorageFindsFilesOnMyIPhone(t *testing.T) {
	t.Parallel()
	out := "group.com.apple.DocumentManager\t/sim/AppGroup/C9F2\n" +
		"group.com.apple.FileProvider.LocalStorage\t/sim/AppGroup/B218\n" +
		"group.com.apple.tipsnext\t/sim/AppGroup/1C23\n"
	dir, ok := localStorage(out)
	assert.True(t, ok)
	assert.Equal(t, "/sim/AppGroup/B218/File Provider Storage", dir)

	_, ok = localStorage("group.com.apple.DocumentManager\t/sim/AppGroup/C9F2\n")
	assert.False(t, ok)
}
