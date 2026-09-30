package main

import "embed"

//go:embed web-ui/dist
var webFS embed.FS

//go:embed migrations
var migrationsFS embed.FS

//go:embed cover
var coverFS embed.FS
