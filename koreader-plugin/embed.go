// Package pluginfiles holds the KOReader plugin's Lua source, which the
// server embeds, zips, and stamps (pkg/koreaderplugin). There is no build
// step: the files under shisho.koplugin are the plugin as KOReader loads it.
package pluginfiles

import "embed"

// Dir is the plugin's directory name, which KOReader requires to end in
// .koplugin. A zip unpacks to this directory inside KOReader's plugins
// folder.
const Dir = "shisho.koplugin"

// FS holds Dir. The all: prefix keeps _meta.lua, which KOReader needs and
// a plain embed would skip.
//
//go:embed all:shisho.koplugin
var FS embed.FS
