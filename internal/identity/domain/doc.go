// Package domain defines the pure Identity account, session, and throttling
// values introduced by Lesson 32. Construction validates shape only; no value
// in this package proves that an HTTP caller supplied a valid credential.
//
// The package deliberately contains no transport, persistence, authorization,
// infrastructure, or raw-secret representation.
package domain
