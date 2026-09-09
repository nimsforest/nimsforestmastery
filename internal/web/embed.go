package web

import "embed"

// templateFS carries the HTML page templates. go:embed is allowed for
// HTML templates only, per the nwc precedent; content ships on disk.
//
//go:embed templates/*
var templateFS embed.FS
