// Package redisstore owns the bounded, secret-safe Redis client used by
// GrowthOS infrastructure adapters. Constructing a Client validates local
// configuration but deliberately performs no network probe: a rebuildable
// cache must not become an application-startup authority.
package redisstore
