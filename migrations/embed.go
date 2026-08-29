// Package migrations embeds immutable, forward-only SQL migrations into the
// migration command binary.
package migrations

import "embed"

// Files contains the migration policy and the sql subtree. Lesson 13
// intentionally ships without a real .up.sql file; the first business schema
// starts in Lesson 18 as 000001_*.up.sql.
//
//go:embed README.md sql
var Files embed.FS
