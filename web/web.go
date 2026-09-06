// Package web embeds the frontend assets into the binary.
package web

import "embed"

// Files holds the entire frontend: html, css, js and assets.
//
//go:embed index.html app.css app.js i18n.js assets
var Files embed.FS
