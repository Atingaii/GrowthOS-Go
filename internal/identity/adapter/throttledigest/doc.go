// Package throttledigest derives non-reversible, dimension-separated MySQL
// throttle keys from canonical login names and trusted socket source addresses.
// It owns no admission policy and never persists the HMAC key or raw subject.
package throttledigest
