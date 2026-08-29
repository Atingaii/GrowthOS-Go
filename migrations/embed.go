// Package migrations embeds immutable, forward-only SQL migrations into the
// migration command binary.
package migrations

import "embed"

// Files contains the migration policy and the sql subtree. Lesson 18 adds the
// first business schema as two single-DDL migrations: Strategy followed by its
// strategy-scoped Awards.
//
//go:embed README.md sql
var Files embed.FS
