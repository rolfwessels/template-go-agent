package agent

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rolfwessels/template-go-agent/internal/config"
)

type fetchPolicyError string

func (e fetchPolicyError) Error() string { return "http_fetch: " + string(e) }

// Conservative special-purpose table based on the IANA IPv4/IPv6 registries:
// https://www.iana.org/assignments/iana-ipv4-special-registry/
// https://www.iana.org/assignments/iana-ipv6-special-registry/
// See README for the categories. Normalize mapped IPv6 BEFORE this table.
var forbiddenPrefixes = func() []netip.Prefix {
	cidrs := []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12",
		"192.0.0.0/24", "192.0.2.0/24", "192.31.196.0/24", "192.52.193.0/24", "192.88.99.0/24", "192.168.0.0/16", "192.175.48.0/24",
		"198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4", "168.63.129.16/32",
		"::/96", "::ffff:0:0/96", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "100:0:0:1::/64",
		"2001::/23", "2001:db8::/32", "2002::/16", "2620:4f:8000::/48", "3fff::/20", "5f00::/16", "fc00::/7", "fe80::/10", "fec0::/10", "ff00::/8",
	}
	prefixes := make([]netip.Prefix, 0, len(cidrs))
	for _, cidr := range cidrs {
		prefixes = append(prefixes, netip.MustParsePrefix(cidr))
	}
	return prefixes
}()

func publicFetchIP(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() {
		return false
	}
	// Only currently allocated IPv6 global unicast space is accepted.
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, prefix := range forbiddenPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

var mandatoryDeniedHosts = []string{
	"localhost", "*.localhost", "*.local", "local", "internal", "*.internal", "*.lan", "lan", "*.home", "home", "*.test", "test", "*.invalid", "invalid",
	"metadata", "metadata.google.internal", "metadata.goog", "*.metadata.goog", "instance-data", "instance-data.ec2.internal", "metadata.azure.internal",
}

func hostMatches(host string, patterns []string) bool {
	for _, pattern := range patterns {
		pattern, _ = config.NormalizeHTTPFetchHost(pattern, true)
		if strings.HasPrefix(pattern, "*.") {
			if strings.HasSuffix(host, pattern[1:]) && host != pattern[2:] {
				return true
			}
		} else if host == pattern {
			return true
		}
	}
	return false
}

func checkFetchHost(host string, p config.HTTPFetchPolicy) error {
	if hostMatches(host, mandatoryDeniedHosts) || hostMatches(host, p.DeniedHosts) {
		return fetchPolicyError("host_denied")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if !publicFetchIP(ip) {
			return fetchPolicyError("address_denied")
		}
	} else if !strings.Contains(host, ".") {
		return fetchPolicyError("host_denied")
	}
	if len(p.AllowedHosts) > 0 && !hostMatches(host, p.AllowedHosts) {
		return fetchPolicyError("host_not_allowed")
	}
	return nil
}

func validateFetchURL(raw string, p config.HTTPFetchPolicy) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.Host == "" || !u.IsAbs() {
		return nil, fetchPolicyError("invalid_url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fetchPolicyError("scheme_denied")
	}
	if u.User != nil {
		return nil, fetchPolicyError("credentials_denied")
	}
	host, err := config.NormalizeHTTPFetchHost(u.Hostname(), false)
	if err != nil {
		return nil, fetchPolicyError("invalid_host")
	}
	// Require brackets precisely for IPv6 literals; reject empty explicit ports.
	isIPv6 := strings.Contains(host, ":")
	if strings.HasPrefix(u.Host, "[") != strings.Contains(u.Hostname(), ":") || strings.HasSuffix(u.Host, ":") {
		return nil, fetchPolicyError("invalid_host")
	}
	port := 80
	if u.Scheme == "https" {
		port = 443
	}
	if value := u.Port(); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != value {
			return nil, fetchPolicyError("port_denied")
		}
		port = n
	}
	if u.Scheme == "http" && port == 443 || u.Scheme == "https" && port == 80 {
		return nil, fetchPolicyError("port_denied")
	}
	allowed := false
	for _, candidate := range p.AllowedPorts {
		if candidate == port {
			allowed = true
		}
	}
	if !allowed {
		return nil, fetchPolicyError("port_denied")
	}
	if err := checkFetchHost(host, p); err != nil {
		return nil, err
	}
	u.Host = host
	if isIPv6 {
		u.Host = "[" + host + "]"
	}
	// Canonical authority avoids alternate numeric forms and trailing-dot bypasses.
	defaultPort := 80
	if u.Scheme == "https" {
		defaultPort = 443
	}
	if port != defaultPort {
		u.Host = net.JoinHostPort(host, strconv.Itoa(port))
	}
	u.Fragment = ""
	return u, nil
}

// These unexported hooks can be supplied explicitly by tests. There is no
// environment/configuration switch that replaces production DNS or dialing.
type fetchNetwork struct {
	lookup func(context.Context, string) ([]netip.Addr, error)
	dial   func(context.Context, string, string) (net.Conn, error)
}

func productionFetchNetwork() fetchNetwork {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	resolver := &net.Resolver{PreferGo: true, StrictErrors: true}
	return fetchNetwork{lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
		return resolver.LookupNetIP(ctx, "ip", host)
	}, dial: dialer.DialContext}
}

// Transport may detach request cancellation while dialing for its pool. Carry
// the original context as a value so DNS and sockets still share its deadline.
type fetchRequestContextKey struct{}

func boundedFetchDialContext(ctx context.Context, maxTimeoutMS int) (context.Context, context.CancelFunc) {
	deadline := time.Now().Add(time.Duration(maxTimeoutMS) * time.Millisecond)
	requestCtx, _ := ctx.Value(fetchRequestContextKey{}).(context.Context)
	if requestCtx != nil {
		if d, ok := requestCtx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
	}
	bounded, cancel := context.WithDeadline(ctx, deadline)
	if requestCtx == nil {
		return bounded, cancel
	}
	stop := context.AfterFunc(requestCtx, cancel)
	if requestCtx.Err() != nil {
		cancel()
	}
	return bounded, func() { stop(); cancel() }
}

func guardedFetchTransport(p config.HTTPFetchPolicy, network fetchNetwork) *http.Transport {
	return &http.Transport{
		Proxy: nil, DialContext: func(ctx context.Context, protocol, address string) (net.Conn, error) {
			ctx, cancel := boundedFetchDialContext(ctx, p.MaxTimeoutMS)
			defer cancel()
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fetchPolicyError("invalid_host")
			}
			host, err = config.NormalizeHTTPFetchHost(host, false)
			if err != nil {
				return nil, fetchPolicyError("invalid_host")
			}
			if err := checkFetchHost(host, p); err != nil {
				return nil, err
			}
			var addresses []netip.Addr
			if ip, err := netip.ParseAddr(host); err == nil {
				addresses = []netip.Addr{ip}
			} else {
				addresses, err = network.lookup(ctx, host)
				if err != nil {
					return nil, fetchPolicyError("dns_failed")
				}
			}
			if len(addresses) == 0 {
				return nil, fetchPolicyError("dns_empty")
			}
			// Validate ALL answers before opening even one socket. No second lookup.
			for _, ip := range addresses {
				if !publicFetchIP(ip) {
					return nil, fetchPolicyError("address_denied")
				}
			}
			for _, ip := range addresses {
				conn, err := network.dial(ctx, protocol, net.JoinHostPort(ip.Unmap().String(), port))
				if err == nil {
					return conn, nil
				}
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
			}
			return nil, fetchPolicyError("dial_failed")
		},
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		MaxResponseHeaderBytes: 64 << 10, MaxIdleConns: 20, MaxIdleConnsPerHost: 2,
		IdleConnTimeout: 30 * time.Second, ForceAttemptHTTP2: true,
	}
}

func safeFetchError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	var policy fetchPolicyError
	if errors.As(err, &policy) {
		return policy
	}
	return fetchPolicyError("request_failed")
}
