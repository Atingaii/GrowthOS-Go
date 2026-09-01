package requestguard

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"

	"github.com/Atingaii/GrowthOS-Go/internal/platform/weborigin"
)

const (
	OriginHeader        = "Origin"
	FetchSiteHeader     = "Sec-Fetch-Site"
	FetchSiteSameOrigin = "same-origin"
)

var (
	ErrInvalidConfiguration = errors.New("identity request guard configuration is invalid")
	ErrOriginRejected       = errors.New("identity request origin is rejected")
	ErrSourceRejected       = errors.New("identity request source is rejected")
)

// Guard is immutable and safe for concurrent use.
type Guard struct {
	publicOrigin string
}

func New(publicOrigin string) (*Guard, error) {
	if !validOrigin(publicOrigin) {
		return nil, ErrInvalidConfiguration
	}
	return &Guard{publicOrigin: publicOrigin}, nil
}

func (guard *Guard) Validate() error {
	if guard == nil || !validOrigin(guard.publicOrigin) {
		return ErrInvalidConfiguration
	}
	return nil
}

func (guard *Guard) PublicOrigin() string {
	if guard == nil {
		return ""
	}
	return guard.publicOrigin
}

func (*Guard) String() string   { return "requestguard.Guard{[REDACTED]}" }
func (*Guard) GoString() string { return "requestguard.Guard{[REDACTED]}" }
func (*Guard) LogValue() slog.Value {
	return slog.StringValue("[REDACTED]")
}
func (*Guard) MarshalJSON() ([]byte, error) { return json.Marshal("[REDACTED]") }

// ValidateUnsafe requires one exact Origin. Fetch Metadata is optional for a
// controlled non-browser client, but if present must appear exactly once and
// be same-origin. It never falls back to Host, Referer, or forwarding headers.
func (guard *Guard) ValidateUnsafe(request *http.Request) error {
	if guard.Validate() != nil || request == nil || !UnsafeMethod(request.Method) {
		return ErrOriginRejected
	}
	origins := request.Header.Values(OriginHeader)
	if len(origins) != 1 || origins[0] != guard.publicOrigin {
		return ErrOriginRejected
	}
	fetchSites := request.Header.Values(FetchSiteHeader)
	if len(fetchSites) == 0 {
		return nil
	}
	if len(fetchSites) != 1 || fetchSites[0] != FetchSiteSameOrigin {
		return ErrOriginRejected
	}
	return nil
}

// TrustedSource ignores all forwarding headers and returns only the parsed
// peer IP from net/http's connected RemoteAddr. Trusting a proxy-derived client
// address requires a future explicit allowlist and a separate decision.
func (guard *Guard) TrustedSource(request *http.Request) (netip.Addr, error) {
	if guard.Validate() != nil || request == nil || request.RemoteAddr == "" {
		return netip.Addr{}, ErrSourceRejected
	}
	host, port, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil || host == "" || port == "" {
		return netip.Addr{}, ErrSourceRejected
	}
	numericPort, err := strconv.Atoi(port)
	if err != nil || numericPort < 1 || numericPort > 65535 || strconv.Itoa(numericPort) != port {
		return netip.Addr{}, ErrSourceRejected
	}
	address, err := netip.ParseAddr(host)
	if err != nil || !address.IsValid() || address.Zone() != "" {
		return netip.Addr{}, ErrSourceRejected
	}
	return address.Unmap(), nil
}

func UnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func validOrigin(value string) bool {
	_, err := weborigin.ParseExact(value)
	return err == nil
}
