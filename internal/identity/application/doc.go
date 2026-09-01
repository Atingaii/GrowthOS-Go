// Package application orchestrates Lesson 32 workforce credential and
// server-side session authentication. It is the only Identity layer allowed to
// turn a strictly restored, active session into a trusted Governance human
// Principal.
//
// The package deliberately owns no HTTP, SQL, Redis, Cookie, CSRF, Role,
// Permission, Scope, Policy, tenant, or business-resource behavior. Adapters
// implement its narrow ports; every failed operation returns zero trusted
// output and no bearer secret.
package application
