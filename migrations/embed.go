// Package migrations embeds immutable, forward-only SQL migrations into the
// migration command binary.
package migrations

import "embed"

// Files contains the migration policy and the sql subtree. Business schema is
// forward-only: Strategy/Award, Strategy routing graph, immutable Strategy
// snapshots, then Marketing Activity publication state currently end at v11.
//
//go:embed README.md sql
var Files embed.FS
