package web

import "embed"

//go:embed all:static
var StaticDir embed.FS
