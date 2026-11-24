package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
