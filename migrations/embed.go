// Package migrations embeds immutable, forward-only SQL migrations into the
// migration command binary.
package migrations

import "embed"

// Files contains the migration policy and the sql subtree. Business schema is
// forward-only: Strategy/Award, Strategy routing graph, immutable Strategy
// snapshots, Marketing Activity publication state, Identity account/session/
// throttle, and the Governance policy plus authorization-audit authority end
// at v22.
//
//go:embed README.md sql
var Files embed.FS
