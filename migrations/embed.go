// Package migrations embeds immutable, forward-only SQL migrations into the
// migration command binary.
package migrations

import "embed"

// Files contains the migration policy and the sql subtree. Business schema is
// forward-only: Strategy/Award, Strategy routing graph, immutable Strategy
// snapshots, Marketing Activity publication state, then Identity account,
// session, and throttle authority currently end at v14.
//
//go:embed README.md sql
var Files embed.FS
