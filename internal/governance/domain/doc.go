// Package domain owns GrowthOS access-control policy language and pure policy
// decisions. Its values describe authorization subjects, protected resources,
// exact actions, role permissions, scoped bindings, immutable policy snapshots,
// and confirmed allow/deny decisions.
//
// Constructing a Principal validates only its shape. It does not authenticate a
// caller. Likewise, Resource tenant and owner values must eventually be loaded
// by a trusted server boundary rather than accepted from a client. This package
// deliberately contains no session, transport, persistence, middleware, UI, or
// business-use-case enforcement.
package domain
