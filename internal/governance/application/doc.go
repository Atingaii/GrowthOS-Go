// Package application coordinates trusted authorization facts around the pure
// Governance policy kernel. It owns neither HTTP credentials nor business
// resource storage: callers must supply a trusted Principal and a Resource
// constructed from the owning business module's server-side facts.
package application
