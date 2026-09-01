// Package mysqlprovisioner implements the create-only MySQL boundary used by
// the trusted workforce-account provisioning command.
//
// It is deliberately separate from mysqlrepo: the long-lived Identity runtime
// account must never acquire credential INSERT privileges merely because the
// one-shot operational adapter exists in the repository.
package mysqlprovisioner
