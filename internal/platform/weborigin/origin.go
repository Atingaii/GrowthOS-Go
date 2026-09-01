// Package weborigin owns the canonical browser Origin serialization accepted
// by configuration, Cookie policy, and unsafe-request enforcement.
package weborigin

import (
	"errors"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

var ErrInvalid = errors.New("web origin is invalid")

// Origin is one exact, browser-canonical HTTP(S) origin.
type Origin struct {
	serialized string
	scheme     string
	loopback   bool
}

// ParseExact rejects values whose browser serialization could differ from the
// configured bytes. In particular, scheme/host case, explicit default ports,
// noncanonical IP spellings, credentials, paths, queries, and fragments are
// not silently normalized.
func ParseExact(value string) (Origin, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return Origin{}, ErrInvalid
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil || parsed.Path != "" ||
		parsed.RawPath != "" || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || parsed.Opaque != "" {
		return Origin{}, ErrInvalid
	}
	hostname := parsed.Hostname()
	if hostname == "" || strings.Contains(hostname, "%") {
		return Origin{}, ErrInvalid
	}

	canonicalHost := ""
	loopback := false
	if address, addressErr := netip.ParseAddr(hostname); addressErr == nil {
		if address.Zone() != "" || address.Is4In6() {
			return Origin{}, ErrInvalid
		}
		loopback = address.IsLoopback()
		if address.Is6() {
			canonicalHost = "[" + address.String() + "]"
		} else {
			canonicalHost = address.String()
		}
	} else {
		if !validCanonicalDNSName(hostname) {
			return Origin{}, ErrInvalid
		}
		canonicalHost = hostname
	}

	port := parsed.Port()
	if port != "" {
		numericPort, portErr := strconv.Atoi(port)
		if portErr != nil || numericPort < 1 || numericPort > 65535 ||
			strconv.Itoa(numericPort) != port ||
			(parsed.Scheme == "http" && numericPort == 80) ||
			(parsed.Scheme == "https" && numericPort == 443) {
			return Origin{}, ErrInvalid
		}
		canonicalHost += ":" + port
	}
	serialized := parsed.Scheme + "://" + canonicalHost
	if value != serialized {
		return Origin{}, ErrInvalid
	}
	return Origin{serialized: serialized, scheme: parsed.Scheme, loopback: loopback}, nil
}

func (origin Origin) String() string { return origin.serialized }

func (origin Origin) Scheme() string { return origin.scheme }

func (origin Origin) IsLoopback() bool { return origin.loopback }

func (origin Origin) Validate() error {
	restored, err := ParseExact(origin.serialized)
	if err != nil || restored != origin {
		return ErrInvalid
	}
	return nil
}

func validCanonicalDNSName(hostname string) bool {
	if len(hostname) == 0 || len(hostname) > 253 || hostname != strings.ToLower(hostname) ||
		strings.HasSuffix(hostname, ".") {
		return false
	}
	labels := strings.Split(hostname, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || !asciiAlphaNumeric(label[0]) ||
			!asciiAlphaNumeric(label[len(label)-1]) {
			return false
		}
		for index := range label {
			character := label[index]
			if !asciiAlphaNumeric(character) && character != '-' {
				return false
			}
		}
	}
	// WHATWG URL parsing treats a numeric final label as a possible IPv4
	// address, including legacy number forms that net/url does not canonicalize.
	// Reject that ambiguity instead of accepting a server string browsers may
	// rewrite to a different Origin.
	last := labels[len(labels)-1]
	if allASCIIDigits(last) {
		return false
	}
	return true
}

func asciiAlphaNumeric(character byte) bool {
	return (character >= 'a' && character <= 'z') ||
		(character >= '0' && character <= '9')
}

func allASCIIDigits(value string) bool {
	if value == "" {
		return false
	}
	for index := range value {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}
