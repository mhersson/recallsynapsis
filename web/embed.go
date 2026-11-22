// Package web embeds the built web app and serves it in front of the API.
package web

import "embed"

// dist holds placeholder.html always, and app/ after a web build.
//
//go:embed all:dist
var dist embed.FS
