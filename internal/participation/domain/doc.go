// Package domain contains Participation-owned business values and decisions.
//
// The first slice deliberately models only new-user eligibility from an
// authoritative registration fact snapshot. It has no dependency on Lottery,
// HTTP, storage, Redis, authentication, or a generic rules engine.
package domain
