package web

import "embed"

// StaticFS embeds all static web assets (CSS, JS) into the binary.
//
//go:embed static/*
var StaticFS embed.FS
