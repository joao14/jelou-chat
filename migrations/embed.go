// Package migrations embebe los archivos .sql para versionarlos junto al binario.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
