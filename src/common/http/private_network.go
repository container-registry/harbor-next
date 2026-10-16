// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package http

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

<<<<<<< HEAD
// privateNetworkAccessEnv opts every Harbor outbound HTTP client that installs the
// public-network guard back into reaching private/loopback/link-local destinations.
// Default (unset/false) fails closed. Set through the chart value network.allowPrivateNetworkAccess
// or the compose HARBOR_ALLOW_PRIVATE_NETWORK_ACCESS variable.
=======
// webhookEgressPolicyEnv selects which destinations the webhook and Slack notification clients
// may reach: allow_all (the default), block_restricted or public_only. See egressLevel.
const webhookEgressPolicyEnv = "HARBOR_WEBHOOK_EGRESS_POLICY"

// webhookEgressAllowlistEnv lists destinations the notification clients may always reach, whatever
// the policy: IPs, CIDRs, hostnames, and "*.domain" suffixes, separated by commas or whitespace. An
// allowlisted hostname is not resolved by Harbor for the check, so it is trusted wherever DNS
// points it.
const webhookEgressAllowlistEnv = "HARBOR_WEBHOOK_EGRESS_ALLOWLIST"

// privateNetworkAccessEnv is the deprecated boolean from 2.15. When the policy is not set it keeps
// its meaning: true allows everything, false allows public destinations only.
>>>>>>> f1b4e3ccc (Fix webhook egress allow all (#24126))
const privateNetworkAccessEnv = "HARBOR_ALLOW_PRIVATE_NETWORK_ACCESS"

// Exceptions and denials follow the IANA IPv4/IPv6 special-purpose address registries.
// A destination is "public" only when it is global-unicast, not private, and not in any
// non-public range below. IsGlobalUnicast() alone is insufficient because it admits
// special-purpose ranges such as 198.18.0.0/15 and fec0::/10.
var (
	publicIPv4Exceptions = []netip.Prefix{
		netip.MustParsePrefix("192.0.0.9/32"),
		netip.MustParsePrefix("192.0.0.10/32"),
	}
	publicIPv6Exceptions = []netip.Prefix{
		netip.MustParsePrefix("2001:1::1/128"),
		netip.MustParsePrefix("2001:1::2/128"),
		netip.MustParsePrefix("2001:1::3/128"),
		netip.MustParsePrefix("2001:3::/32"),
		netip.MustParsePrefix("2001:4:112::/48"),
		netip.MustParsePrefix("2001:20::/28"),
		netip.MustParsePrefix("2001:30::/28"),
	}
	publicIPv6Prefix  = netip.MustParsePrefix("2000::/3")
	nonPublicPrefixes = []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("192.88.99.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("224.0.0.0/4"),
		netip.MustParsePrefix("240.0.0.0/4"),
		netip.MustParsePrefix("::/96"),
		netip.MustParsePrefix("64:ff9b::/96"),
		netip.MustParsePrefix("64:ff9b:1::/48"),
		netip.MustParsePrefix("100::/64"),
		netip.MustParsePrefix("100:0:0:1::/64"),
		netip.MustParsePrefix("2001::/23"),
		netip.MustParsePrefix("2001:db8::/32"),
		netip.MustParsePrefix("2002::/16"),
		netip.MustParsePrefix("3fff::/20"),
		netip.MustParsePrefix("5f00::/16"),
		netip.MustParsePrefix("fc00::/7"),
		netip.MustParsePrefix("fe80::/10"),
		netip.MustParsePrefix("fec0::/10"),
		netip.MustParsePrefix("ff00::/8"),
	}
	metadataTargetHosts = map[string]struct{}{
		"metadata.azure.com":       {},
		"metadata.google.internal": {},
	}
)

func newGuardedDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
}

// privateNetworkAccessAllowed reports whether the operator opted back into private egress.
func privateNetworkAccessAllowed() bool {
	allowed, err := strconv.ParseBool(os.Getenv(privateNetworkAccessEnv))
	return err == nil && allowed
}

<<<<<<< HEAD
// blockPrivateNetwork is a net.Dialer.Control callback. It runs after DNS resolution and on
// every dial (including redirect hops), so it defeats DNS rebinding. It fails closed unless
// HARBOR_ALLOW_PRIVATE_NETWORK_ACCESS is set.
func blockPrivateNetwork(_ string, address string, _ syscall.RawConn) error {
	if privateNetworkAccessAllowed() {
=======
func (level egressLevel) String() string {
	return egressLevelNames[level]
}

// egressPolicy decides which destinations the notification clients may reach. The allowlist
// overrides the level.
type egressPolicy struct {
	level           egressLevel
	allowedPrefixes []netip.Prefix
	// allowedHosts holds exact hostnames; entries starting with "." match any subdomain.
	allowedHosts []string
}

var egressPolicyCache struct {
	sync.Mutex
	loaded                    bool
	policyEnv, legacyEnv, raw string
	policy                    egressPolicy
}

// currentEgressPolicy reads the policy from the environment, re-parsing only when it changed so
// that invalid and deprecated values are logged once rather than on every dial.
func currentEgressPolicy() egressPolicy {
	policyEnv, legacyEnv, allowlist := os.Getenv(webhookEgressPolicyEnv), os.Getenv(privateNetworkAccessEnv), os.Getenv(webhookEgressAllowlistEnv)
	egressPolicyCache.Lock()
	defer egressPolicyCache.Unlock()
	cache := &egressPolicyCache
	if !cache.loaded || cache.policyEnv != policyEnv || cache.legacyEnv != legacyEnv || cache.raw != allowlist {
		cache.policy = parseEgressPolicy(policyEnv, legacyEnv, allowlist)
		cache.policyEnv, cache.legacyEnv, cache.raw = policyEnv, legacyEnv, allowlist
		cache.loaded = true
	}
	return cache.policy
}

func parseEgressPolicy(policyEnv, legacyEnv, allowlist string) egressPolicy {
	policy := egressPolicy{level: parseEgressLevel(policyEnv, legacyEnv)}
	for _, entry := range strings.FieldsFunc(allowlist, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
		entry = strings.TrimSuffix(strings.ToLower(entry), ".")
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			policy.allowedPrefixes = append(policy.allowedPrefixes, netip.PrefixFrom(prefix.Addr().Unmap(), unmappedBits(prefix)).Masked())
			continue
		}
		if address, err := netip.ParseAddr(entry); err == nil && address.Zone() == "" {
			address = address.Unmap()
			policy.allowedPrefixes = append(policy.allowedPrefixes, netip.PrefixFrom(address, address.BitLen()))
			continue
		}
		// only "*.domain" is a wildcard; it is stored as ".domain"
		host, isWildcard := strings.CutPrefix(entry, "*.")
		if !isValidAllowlistHost(host) {
			log.Warningf("ignoring invalid %s entry %q", webhookEgressAllowlistEnv, entry)
			continue
		}
		if isWildcard {
			host = "." + host
		}
		policy.allowedHosts = append(policy.allowedHosts, host)
	}
	return policy
}

// parseEgressLevel applies the policy, then the deprecated boolean, then the default. An invalid
// value falls back to public_only so that a typo can never widen egress.
func parseEgressLevel(policyEnv, legacyEnv string) egressLevel {
	if value := strings.ToLower(strings.TrimSpace(policyEnv)); value != "" {
		for level, name := range egressLevelNames {
			if value == name {
				return level
			}
		}
		log.Warningf("invalid %s=%q, allowing public webhook targets only", webhookEgressPolicyEnv, policyEnv)
		return egressPublicOnly
	}
	value := strings.TrimSpace(legacyEnv)
	if value == "" {
		return egressAllowAll
	}
	allowed, err := strconv.ParseBool(value)
	switch {
	case err != nil:
		log.Warningf("invalid %s=%q, allowing public webhook targets only", privateNetworkAccessEnv, legacyEnv)
		return egressPublicOnly
	case allowed:
		log.Warningf("%s is deprecated, use %s=%s", privateNetworkAccessEnv, webhookEgressPolicyEnv, egressAllowAll)
		return egressAllowAll
	default:
		log.Warningf("%s is deprecated, use %s=%s", privateNetworkAccessEnv, webhookEgressPolicyEnv, egressPublicOnly)
		return egressPublicOnly
	}
}

// unmappedBits keeps an IPv4-mapped prefix such as ::ffff:10.0.0.0/104 equivalent to 10.0.0.0/8.
func unmappedBits(prefix netip.Prefix) int {
	if prefix.Addr().Is4In6() {
		return max(prefix.Bits()-96, 0)
	}
	return prefix.Bits()
}

// isValidAllowlistHost accepts DNS names only. The last label must not be numeric, so an IP
// address can never enter the list as a hostname and skip address classification.
func isValidAllowlistHost(host string) bool {
	labels := strings.Split(host, ".")
	if host == "" || strings.Trim(labels[len(labels)-1], "0123456789") == "" {
		return false
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
				return false
			}
		}
	}
	return true
}

// allowsHost reports whether host is on the allowlist. host must be lower-case without a
// trailing dot.
func (policy egressPolicy) allowsHost(host string) bool {
	for _, allowed := range policy.allowedHosts {
		if host == allowed || strings.HasPrefix(allowed, ".") && strings.HasSuffix(host, allowed) {
			return true
		}
	}
	return false
}

// checkAddress returns an error when the policy blocks address.
func (policy egressPolicy) checkAddress(address netip.Addr) error {
	address = address.Unmap()
	switch {
	case policy.level == egressAllowAll, containsAddress(policy.allowedPrefixes, address.WithZone("")):
>>>>>>> f1b4e3ccc (Fix webhook egress allow all (#24126))
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("failed to parse dial address %q: %w", address, err)
	}
	addressIP, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("failed to parse dial IP %q: %w", host, err)
	}
	addressIP = addressIP.Unmap()
	if !isPublicNetworkAddress(addressIP) {
		return fmt.Errorf("connections to private network address %s are blocked", addressIP)
	}
	return nil
}

func isPublicNetworkAddress(address netip.Addr) bool {
	if !address.IsValid() || address.Zone() != "" {
		return false
	}

	address = address.Unmap()
	if address.Is4() && containsAddress(publicIPv4Exceptions, address) {
		return true
	}
	if address.Is6() {
		if containsAddress(publicIPv6Exceptions, address) {
			return true
		}
		if !publicIPv6Prefix.Contains(address) {
			return false
		}
	}
	if !address.IsGlobalUnicast() || address.IsPrivate() {
		return false
	}

	return !containsAddress(nonPublicPrefixes, address)
}

func containsAddress(prefixes []netip.Prefix, address netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

// NetworkResolver resolves target hostnames for public-network validation.
type NetworkResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

// ValidatePublicNetworkTarget rejects a target host that does not resolve entirely to public
// addresses. Literal IPs are classified directly; hostnames are resolved and every returned
// address must be public. It short-circuits when private access is allowed so the check matches
// the dial-time guard's escape hatch.
func ValidatePublicNetworkTarget(ctx context.Context, resolver NetworkResolver, host string) error {
	if privateNetworkAccessAllowed() {
		return nil
	}
	_, err := resolvePublicNetworkTarget(ctx, resolver, host)
	return err
}

// resolvePublicNetworkTarget returns the first address of a host whose addresses are all public.
func resolvePublicNetworkTarget(ctx context.Context, resolver NetworkResolver, host string) (netip.Addr, error) {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" {
		return netip.Addr{}, fmt.Errorf("target must include a hostname")
	}
	if isBlockedTargetHostname(host) {
		return netip.Addr{}, fmt.Errorf("target hostname %q is not public", host)
	}

	address, err := netip.ParseAddr(host)
	if err == nil {
		if !isPublicNetworkAddress(address) {
			return netip.Addr{}, fmt.Errorf("target address %q is not public", host)
		}
		return address, nil
	}

	if resolver == nil {
		return netip.Addr{}, fmt.Errorf("target hostname %q cannot be resolved", host)
	}
	addresses, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return netip.Addr{}, fmt.Errorf("target hostname %q cannot be resolved", host)
	}
	for _, address := range addresses {
		if !isPublicNetworkAddress(address) {
			return netip.Addr{}, fmt.Errorf("target hostname %q resolves to a non-public address", host)
		}
	}
	return addresses[0].Unmap(), nil
}

func isBlockedTargetHostname(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	for metadataHost := range metadataTargetHosts {
		if host == metadataHost || strings.HasSuffix(host, "."+metadataHost) {
			return true
		}
	}
	return false
}

// withPublicNetworkOnly blocks connections to non-public destinations. Direct connections are
// checked at dial time on the resolved address. When the transport's proxy
// (HTTP_PROXY/HTTPS_PROXY/NO_PROXY) applies, the dial goes to the operator's proxy instead, so
// the target host is validated before the request is handed to it. Honors the
// HARBOR_ALLOW_PRIVATE_NETWORK_ACCESS escape hatch.
func withPublicNetworkOnly(resolver NetworkResolver) func(*http.Transport) {
	return func(transport *http.Transport) {
		guard := &publicNetworkGuard{resolver: resolver, upstreamProxy: transport.Proxy}
		dialer := newGuardedDialer()
		dialer.Control = blockPrivateNetwork
		guard.guardedDial = dialer.DialContext
		guard.proxyDial = newGuardedDialer().DialContext
		if guard.upstreamProxy != nil {
			transport.Proxy = guard.proxy
		}
		transport.DialContext = guard.dialContext
	}
}

// NewPublicNetworkTransport returns a transport that only reaches public destinations, for the
// notification (webhook/slack) clients. On top of the dial-time guard it pins proxied requests
// to the address Harbor validated: the proxy receives that IP instead of the hostname, so it
// cannot resolve the name again to a private address (DNS rebinding). HTTPS tunnels to the IP
// and still sends the original Host and verifies the certificate against the hostname. Plain
// HTTP through a proxy sends the IP as Host, because the proxy reads the target from it.
func NewPublicNetworkTransport(opts ...func(*http.Transport)) http.RoundTripper {
	return newPublicNetworkTransport(net.DefaultResolver, opts...)
}

func newPublicNetworkTransport(resolver NetworkResolver, opts ...func(*http.Transport)) http.RoundTripper {
	transport := newDefaultTransport()
	for _, opt := range opts {
		opt(transport)
	}
	upstreamProxy := transport.Proxy
	withPublicNetworkOnly(resolver)(transport)
	return &pinnedProxyTransport{base: transport, upstreamProxy: upstreamProxy, resolver: resolver}
}

type pinnedProxyTransport struct {
	base          *http.Transport
	upstreamProxy func(*http.Request) (*url.URL, error)
	resolver      NetworkResolver
	// tlsTransports holds a clone of base per HTTPS hostname, with ServerName set so the
	// certificate is verified against the hostname while the tunnel goes to the pinned IP.
	tlsTransports sync.Map
}

func (pinned *pinnedProxyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if pinned.upstreamProxy == nil || privateNetworkAccessAllowed() {
		return pinned.base.RoundTrip(req)
	}
	proxyURL, err := pinned.upstreamProxy(req)
	if err != nil {
		closeRequestBody(req)
		return nil, err
	}
	hostname := req.URL.Hostname()
	if _, err := netip.ParseAddr(hostname); proxyURL == nil || err == nil {
		// direct requests are checked at dial time, and a literal IP has nothing to rebind
		return pinned.base.RoundTrip(req)
	}

	address, err := resolvePublicNetworkTarget(req.Context(), pinned.resolver, hostname)
	if err != nil {
		closeRequestBody(req)
		return nil, err
	}
	pinnedReq := req.Clone(req.Context())
	switch port := req.URL.Port(); {
	case port != "":
		pinnedReq.URL.Host = net.JoinHostPort(address.String(), port)
	case address.Is6():
		pinnedReq.URL.Host = "[" + address.String() + "]"
	default:
		pinnedReq.URL.Host = address.String()
	}
	transport := pinned.base
	if strings.EqualFold(req.URL.Scheme, "https") {
		// the tunnel goes to the IP; the request inside it keeps the original Host
		if pinnedReq.Host == "" {
			pinnedReq.Host = req.URL.Host
		}
		transport = pinned.tlsTransport(hostname)
	} else {
		// a plain HTTP proxy request carries Host in its absolute URI, which the proxy would
		// resolve again, so the target sees the IP as Host
		pinnedReq.Host = pinnedReq.URL.Host
	}
	resp, err := transport.RoundTrip(pinnedReq)
	if resp != nil {
		resp.Request = req
	}
	return resp, err
}

// Base returns the guarded transport underneath the pinning, for inspection.
func (pinned *pinnedProxyTransport) Base() *http.Transport {
	return pinned.base
}

func (pinned *pinnedProxyTransport) tlsTransport(hostname string) *http.Transport {
	if transport, ok := pinned.tlsTransports.Load(hostname); ok {
		return transport.(*http.Transport)
	}
	transport := pinned.base.Clone()
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.ServerName = hostname
	actual, _ := pinned.tlsTransports.LoadOrStore(hostname, transport)
	return actual.(*http.Transport)
}

// closeRequestBody honours the RoundTripper contract of closing the body on error.
func closeRequestBody(req *http.Request) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
}

type publicNetworkGuard struct {
	resolver      NetworkResolver
	upstreamProxy func(*http.Request) (*url.URL, error)
	guardedDial   func(context.Context, string, string) (net.Conn, error)
	proxyDial     func(context.Context, string, string) (net.Conn, error)
	// proxyAddrs holds the host:port of every proxy the upstream function returned. Only dials to
	// exactly these addresses skip the private-address check: the proxy is operator configuration
	// and is commonly on a private network.
	proxyAddrs sync.Map
}

func (guard *publicNetworkGuard) proxy(req *http.Request) (*url.URL, error) {
	proxyURL, err := guard.upstreamProxy(req)
	if err != nil || privateNetworkAccessAllowed() {
		return proxyURL, err
	}
	if proxyURL == nil {
		// A direct request whose target is the proxy's own address would otherwise inherit the
		// proxy's dial exemption.
		if _, isProxy := guard.proxyAddrs.Load(canonicalAddr(req.URL)); isProxy {
			return nil, fmt.Errorf("connections to private network address %s are blocked", req.URL.Host)
		}
		return nil, nil //nolint:nilnil // http.Transport.Proxy reads a nil URL and nil error as "no proxy"
	}
	// The proxy, not this process, resolves and connects to the target, so the dial-time check
	// cannot see it. Validate the target before handing it over.
	if err := ValidatePublicNetworkTarget(req.Context(), guard.resolver, req.URL.Hostname()); err != nil {
		return nil, err
	}
	guard.proxyAddrs.Store(canonicalAddr(proxyURL), struct{}{})
	return proxyURL, nil
}

func (guard *publicNetworkGuard) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if _, isProxy := guard.proxyAddrs.Load(address); isProxy {
		return guard.proxyDial(ctx, network, address)
	}
	return guard.guardedDial(ctx, network, address)
}

var defaultPorts = map[string]string{"http": "80", "https": "443", "socks5": "1080", "socks5h": "1080"}

// canonicalAddr mirrors net/http's host:port form for a URL, which is the address the transport
// passes to DialContext.
func canonicalAddr(target *url.URL) string {
	port := target.Port()
	if port == "" {
		port = defaultPorts[strings.ToLower(target.Scheme)]
	}
	return net.JoinHostPort(target.Hostname(), port)
}
