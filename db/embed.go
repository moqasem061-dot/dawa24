// Package db embeds the SQL migrations into the binary.
//
// Embedding means the deployed image carries the exact migrations it was built
// with. There is no separate migration artifact to get out of step with the
// code, and no possibility of running yesterday's schema against today's binary.
package db

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS
