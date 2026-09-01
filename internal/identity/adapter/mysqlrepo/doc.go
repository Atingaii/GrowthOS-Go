// Package mysqlrepo implements the authoritative MySQL persistence ports for
// workforce credentials, authentication admission, and server-side sessions.
//
// The adapter deliberately owns no authorization vocabulary. It restores the
// Identity domain strictly and fails closed when stored state cannot prove the
// frozen session or throttle invariants.
package mysqlrepo
