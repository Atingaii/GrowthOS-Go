// Package passwordhash implements the local workforce password boundary.
//
// It deliberately owns only password enrollment and verification. Account
// lookup, authentication failure disclosure, throttling, and credential
// persistence belong to the Identity application and persistence adapters.
package passwordhash
