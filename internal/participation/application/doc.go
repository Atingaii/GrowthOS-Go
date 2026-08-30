// Package application coordinates Participation use cases through
// consumer-owned ports.
//
// Lesson 25 exposes one concrete new-user eligibility service. It does not
// define a generic rule interface, own an external user directory, or compose
// the decision into the unauthenticated Lottery demonstration route.
package application
