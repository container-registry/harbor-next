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
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubNetworkResolver struct {
	answers map[string][]netip.Addr
	errors  map[string]error
	calls   []string
}

func (resolver *stubNetworkResolver) LookupNetIP(_ context.Context, _ string, host string) ([]netip.Addr, error) {
	resolver.calls = append(resolver.calls, host)
	if err := resolver.errors[host]; err != nil {
		return nil, err
	}
	return resolver.answers[host], nil
}

func TestValidatePublicNetworkTarget(t *testing.T) {
	t.Setenv(privateNetworkAccessEnv, "false")
	resolver := &stubNetworkResolver{
		answers: map[string][]netip.Addr{
			"example.com": {netip.MustParseAddr("8.8.8.8")},
			"mixed.example.com": {
				netip.MustParseAddr("8.8.8.8"),
				netip.MustParseAddr("127.0.0.1"),
			},
		},
		errors: map[string]error{
			"failure.example.com": errors.New("resolver failed"),
		},
	}

	for _, host := range []string{
		"example.com",
		"example.com.",
		"8.8.8.8",
		"192.0.0.9",
		"192.0.0.10",
		"2001:1::1",
		"2606:4700:4700::1111",
	} {
		t.Run("allows "+host, func(t *testing.T) {
			require.NoError(t, ValidatePublicNetworkTarget(context.Background(), resolver, host))
		})
	}

	for _, host := range []string{
		"",
		"localhost",
		"api.localhost.",
		"metadata.google.internal",
		"sub.metadata.azure.com.",
		"mixed.example.com",
		"missing.example.com",
		"failure.example.com",
		"0.0.0.0",
		"10.0.0.1",
		"100.64.0.1",
		"127.0.0.1",
		"169.254.169.254",
		"172.16.0.1",
		"192.168.0.1",
		"198.18.0.1",
		"224.0.0.1",
		"240.0.0.1",
		"::1",
		"::ffff:127.0.0.1",
		"fc00::1",
		"fe80::1",
		"fe80::1%eth0",
	} {
		t.Run("blocks "+host, func(t *testing.T) {
			require.Error(t, ValidatePublicNetworkTarget(context.Background(), resolver, host))
		})
	}
}

<<<<<<< HEAD
// TestValidatePublicNetworkTargetEscapeHatch proves the HARBOR_ALLOW_PRIVATE_NETWORK_ACCESS
// opt-out lets an operator keep webhooking to internal targets (backward-compat contract).
func TestValidatePublicNetworkTargetEscapeHatch(t *testing.T) {
	t.Setenv(privateNetworkAccessEnv, "false")
	require.Error(t, ValidatePublicNetworkTarget(context.Background(), nil, "127.0.0.1"))
	t.Setenv(privateNetworkAccessEnv, "true")
	require.NoError(t, ValidatePublicNetworkTarget(context.Background(), nil, "127.0.0.1"))
}

func TestWithPublicNetworkOnly(t *testing.T) {
	t.Setenv(privateNetworkAccessEnv, "false")
=======
// TestValidateNetworkTargetDefault covers the default policy (allow_all): all targets are allowed.
func TestValidateNetworkTargetDefault(t *testing.T) {
	setEgressEnv(t, "", "", "")
	resolver := &stubNetworkResolver{answers: map[string][]netip.Addr{
		"rebind.example.com": {netip.MustParseAddr("169.254.169.254")},
	}}

	for _, host := range []string{
		"8.8.8.8",
		"10.0.0.1",
		"127.0.0.1",
		"169.254.169.254",
		"localhost",
		"metadata.google.internal",
		"rebind.example.com",
	} {
		t.Run("allows "+host, func(t *testing.T) {
			require.NoError(t, ValidateNetworkTarget(context.Background(), resolver, host))
		})
	}
}

// TestValidateNetworkTargetBlockRestricted covers the block_restricted policy: internal webhook receivers keep
// working, while targets that reach the host itself or cloud metadata stay blocked.
func TestValidateNetworkTargetBlockRestricted(t *testing.T) {
	setEgressEnv(t, "block_restricted", "", "")
	resolver := &stubNetworkResolver{answers: map[string][]netip.Addr{
		"receiver.ns.svc.cluster.local": {netip.MustParseAddr("10.96.12.7")},
		"rebind.example.com":            {netip.MustParseAddr("169.254.169.254")},
	}}

	for _, host := range []string{
		"receiver.ns.svc.cluster.local",
		"8.8.8.8",
		"10.0.0.1",
		"100.64.0.1",
		"172.16.0.1",
		"192.168.0.1",
		"198.18.0.1",
		"fc00::1",
		"fd12:3456::1",
	} {
		t.Run("allows "+host, func(t *testing.T) {
			require.NoError(t, ValidateNetworkTarget(context.Background(), resolver, host))
		})
	}

	for _, host := range []string{
		"localhost",
		"metadata.google.internal",
		"rebind.example.com",
		"0.0.0.0",
		"127.0.0.1",
		"169.254.169.254",
		"100.100.100.200",
		"168.63.129.16",
		"224.0.0.1",
		"255.255.255.255",
		"::",
		"::1",
		"::ffff:169.254.169.254",
		"::169.254.169.254",
		"64:ff9b::a9fe:a9fe",
		"2002:a9fe:a9fe::1",
		"2001:0:a9fe:a9fe::1",
		"fd00:ec2::254",
		"fd20:ce::254",
		"fe80::1",
		"fe80::1%eth0",
		"ff02::1",
	} {
		t.Run("blocks "+host, func(t *testing.T) {
			require.ErrorContains(t, ValidateNetworkTarget(context.Background(), resolver, host), "restricted")
		})
	}
}

// TestEgressLevelPrecedence pins the compatibility contract: the policy wins, the deprecated boolean
// keeps its 2.15 meaning when the policy is unset, and invalid values fail closed.
func TestEgressLevelPrecedence(t *testing.T) {
	for _, tc := range []struct {
		policy, legacy string
		want           egressLevel
	}{
		{"", "", egressAllowAll},
		{" ", " ", egressAllowAll},
		{"", "true", egressAllowAll},
		{"", "1", egressAllowAll},
		{"", "false", egressPublicOnly},
		{"", "flase", egressPublicOnly},
		{"allow_all", "", egressAllowAll},
		{"Block_Restricted", "", egressBlockRestricted},
		{"public_only", "", egressPublicOnly},
		{"public_only", "true", egressPublicOnly},
		{"allow_all", "false", egressAllowAll},
		{"allow-all", "true", egressPublicOnly},
	} {
		t.Run(tc.policy+"/"+tc.legacy, func(t *testing.T) {
			assert.Equal(t, tc.want, parseEgressLevel(tc.policy, tc.legacy))
		})
	}
}

// TestValidateNetworkTargetPerLevel covers what each level lets through.
func TestValidateNetworkTargetPerLevel(t *testing.T) {
	for _, tc := range []struct {
		policy, legacy              string
		public, private, restricted bool
	}{
		{"allow_all", "", true, true, true},
		{"", "true", true, true, true},
		{"block_restricted", "", true, true, false},
		{"", "", true, true, true},
		{"public_only", "", true, false, false},
		{"", "false", true, false, false},
	} {
		t.Run(tc.policy+"/"+tc.legacy, func(t *testing.T) {
			setEgressEnv(t, tc.policy, tc.legacy, "")
			for target, allowed := range map[string]bool{
				"8.8.8.8":                  tc.public,
				"10.0.0.1":                 tc.private,
				"fd12::1":                  tc.private,
				"169.254.169.254":          tc.restricted,
				"127.0.0.1":                tc.restricted,
				"metadata.google.internal": tc.restricted,
			} {
				err := ValidateNetworkTarget(context.Background(), nil, target)
				if allowed {
					assert.NoError(t, err, target)
				} else {
					assert.Error(t, err, target)
				}
				if address, parseErr := netip.ParseAddr(target); parseErr == nil {
					err := checkEgress("tcp", net.JoinHostPort(address.String(), "443"), nil)
					assert.Equal(t, allowed, err == nil, "dial %s: %v", target, err)
				}
			}
		})
	}
}

// TestValidateNetworkTargetAllowlist covers HARBOR_PRIVATE_NETWORK_ALLOWLIST: listed destinations
// pass even with private access disabled, including restricted ones the operator names
// explicitly, and allowlisted hostnames are trusted without resolution.
func TestValidateNetworkTargetAllowlist(t *testing.T) {
	setEgressEnv(t, "public_only", "", "10.1.0.0/16, hooks.internal *.svc.cluster.local,127.0.0.1,::ffff:192.168.0.0/112,not_a_host!,*.")
	resolver := &stubNetworkResolver{answers: map[string][]netip.Addr{
		"listed-range.example.com": {netip.MustParseAddr("10.1.4.4")},
		"other-range.example.com":  {netip.MustParseAddr("10.2.4.4")},
	}}

	for _, host := range []string{
		"10.1.2.3",
		"listed-range.example.com",
		"hooks.internal",
		"HOOKS.internal.",
		"receiver.ns.svc.cluster.local",
		"127.0.0.1",
		"192.168.7.7",
		"8.8.8.8",
	} {
		t.Run("allows "+host, func(t *testing.T) {
			require.NoError(t, ValidateNetworkTarget(context.Background(), resolver, host))
		})
	}
	for _, host := range []string{
		"10.2.0.1",
		"other-range.example.com",
		"svc.cluster.local",
		"evilhooks.internal",
		"127.0.0.2",
		"169.254.169.254",
	} {
		t.Run("blocks "+host, func(t *testing.T) {
			require.Error(t, ValidateNetworkTarget(context.Background(), resolver, host))
		})
	}
	assert.NotContains(t, resolver.calls, "hooks.internal", "allowlisted hostnames are not resolved")
}

func TestCheckEgress(t *testing.T) {
	setEgressEnv(t, "block_restricted", "", "")
	require.NoError(t, checkEgress("tcp4", "10.0.0.1:443", nil))
	require.NoError(t, checkEgress("tcp6", "[fd12::1]:443", nil))
	require.ErrorContains(t, checkEgress("tcp4", "169.254.169.254:80", nil), "restricted network address")
	require.ErrorContains(t, checkEgress("tcp6", "[::ffff:127.0.0.1]:80", nil), "restricted network address")

	setEgressEnv(t, "public_only", "", "")
	require.ErrorContains(t, checkEgress("tcp4", "10.0.0.1:443", nil), "private network address")
	require.NoError(t, checkEgress("tcp4", "8.8.8.8:443", nil))

	setEgressEnv(t, "", "", "")
	require.NoError(t, checkEgress("tcp4", "10.0.0.1:443", nil))
	require.NoError(t, checkEgress("tcp4", "169.254.169.254:80", nil))
	require.NoError(t, checkEgress("tcp4", "127.0.0.1:80", nil))
}

func TestWithEgressGuard(t *testing.T) {
	setEgressEnv(t, "block_restricted", "", "")
>>>>>>> f1b4e3ccc (Fix webhook egress allow all (#24126))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	dial := func() error {
		transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
		withPublicNetworkOnly(net.DefaultResolver)(transport)
		conn, err := transport.DialContext(context.Background(), "tcp", listener.Addr().String())
		if conn != nil {
			_ = conn.Close()
		}
		return err
	}

	// Default: the loopback dial is refused before the connection is made.
	require.ErrorContains(t, dial(), "private network address")

	// Escape hatch: the same dial succeeds when private access is allowed.
	t.Setenv(privateNetworkAccessEnv, "true")
	require.NoError(t, dial())
}

type recordingProxy struct {
	mu       sync.Mutex
	requests []string
	url      *url.URL
}

func newRecordingProxy(t *testing.T) *recordingProxy {
	t.Helper()
	proxy := &recordingProxy{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxy.mu.Lock()
		proxy.requests = append(proxy.requests, r.URL.String())
		proxy.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	proxyURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	proxy.url = proxyURL
	return proxy
}

func (proxy *recordingProxy) seen() []string {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	return append([]string(nil), proxy.requests...)
}

// TestWithPublicNetworkOnlyHonoursProxy covers the jobservice HTTP(S)_PROXY path: a public target
// must still be sent through the configured proxy, even though the proxy itself sits on a
// private address that the dial-time guard would otherwise refuse.
func TestWithPublicNetworkOnlyHonoursProxy(t *testing.T) {
	t.Setenv(privateNetworkAccessEnv, "false")
	proxy := newRecordingProxy(t)

	transport := &http.Transport{Proxy: http.ProxyURL(proxy.url)}
	withPublicNetworkOnly(net.DefaultResolver)(transport)
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}

	resp, err := client.Get("http://8.8.8.8/hook")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, []string{"http://8.8.8.8/hook"}, proxy.seen())
}

// TestWithPublicNetworkOnlyProxyBlocksPrivateTargets proves that routing through a proxy does not
// reopen the SSRF: non-public targets are refused before the proxy is ever contacted.
func TestWithPublicNetworkOnlyProxyBlocksPrivateTargets(t *testing.T) {
	t.Setenv(privateNetworkAccessEnv, "false")
	proxy := newRecordingProxy(t)
	resolver := &stubNetworkResolver{answers: map[string][]netip.Addr{
		"public.example.com":  {netip.MustParseAddr("8.8.8.8")},
		"rebound.example.com": {netip.MustParseAddr("10.0.0.1")},
	}}

	transport := &http.Transport{Proxy: http.ProxyURL(proxy.url)}
	withPublicNetworkOnly(resolver)(transport)
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}

	for _, target := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/",
		"https://[fd00::1]/",
		"http://metadata.google.internal/",
		"http://rebound.example.com/",
		"http://unresolvable.example.com/",
	} {
		t.Run(target, func(t *testing.T) {
			_, err := client.Get(target)
			require.Error(t, err)
		})
	}
	assert.Empty(t, proxy.seen(), "no private target may reach the proxy")

	resp, err := client.Get("http://public.example.com/hook")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, []string{"http://public.example.com/hook"}, proxy.seen())

}

<<<<<<< HEAD
// TestWithPublicNetworkOnlyProxyAddressNotReachableDirectly covers a NO_PROXY entry that matches
// the proxy itself: a direct request to the proxy's address must not inherit the dial exemption
// the proxy gets once it has been used.
func TestWithPublicNetworkOnlyProxyAddressNotReachableDirectly(t *testing.T) {
	t.Setenv(privateNetworkAccessEnv, "false")
=======
// TestWithEgressGuardProxyAddressNotReachableDirectly covers a NO_PROXY entry that matches the
// proxy itself: a direct request to the proxy's address must not inherit the dial exemption the
// proxy gets once it has been used.
func TestWithEgressGuardProxyAddressNotReachableDirectly(t *testing.T) {
	setEgressEnv(t, "block_restricted", "", "")
>>>>>>> f1b4e3ccc (Fix webhook egress allow all (#24126))
	proxy := newRecordingProxy(t)
	transport := &http.Transport{Proxy: func(req *http.Request) (*url.URL, error) {
		if req.URL.Host == proxy.url.Host {
			return nil, nil
		}
		return proxy.url, nil
	}}
	withPublicNetworkOnly(&stubNetworkResolver{})(transport)
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}

	resp, err := client.Get("http://8.8.8.8/hook")
	require.NoError(t, err)
	_ = resp.Body.Close()

	_, err = client.Get(proxy.url.String() + "/internal")
	require.ErrorContains(t, err, "private network address")
	assert.Equal(t, []string{"http://8.8.8.8/hook"}, proxy.seen())
}

<<<<<<< HEAD
// TestWithPublicNetworkOnlyNoProxyStillGuardsDial covers NO_PROXY: when the proxy function
// declines a target, the direct dial keeps the resolved-address check.
func TestWithPublicNetworkOnlyNoProxyStillGuardsDial(t *testing.T) {
	t.Setenv(privateNetworkAccessEnv, "false")
=======
// TestWithEgressGuardNoProxyStillGuardsDial covers NO_PROXY: when the proxy function declines a
// target, the direct dial keeps the resolved-address check.
func TestWithEgressGuardNoProxyStillGuardsDial(t *testing.T) {
	setEgressEnv(t, "block_restricted", "", "")
>>>>>>> f1b4e3ccc (Fix webhook egress allow all (#24126))
	target := newRecordingProxy(t)
	transport := &http.Transport{Proxy: func(*http.Request) (*url.URL, error) { return nil, nil }}
	withPublicNetworkOnly(&stubNetworkResolver{})(transport)
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}

	_, err := client.Get(target.url.String())
	require.ErrorContains(t, err, "private network address")
	assert.Empty(t, target.seen())
}

// TestWithPublicNetworkOnlyProxyEscapeHatch keeps the opt-out working on the proxied path.
func TestWithPublicNetworkOnlyProxyEscapeHatch(t *testing.T) {
	t.Setenv(privateNetworkAccessEnv, "true")
	proxy := newRecordingProxy(t)
	transport := &http.Transport{Proxy: http.ProxyURL(proxy.url)}
	withPublicNetworkOnly(&stubNetworkResolver{})(transport)
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}

	resp, err := client.Get("http://10.0.0.1/hook")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, []string{"http://10.0.0.1/hook"}, proxy.seen())
}

<<<<<<< HEAD
// TestPublicNetworkTransportPinsProxiedHTTPTarget covers DNS rebinding through a proxy: the proxy
// receives the address Harbor validated, never the hostname, so it cannot resolve it again to a
// private address. Plain HTTP carries that address as Host too.
func TestPublicNetworkTransportPinsProxiedHTTPTarget(t *testing.T) {
	t.Setenv(privateNetworkAccessEnv, "false")
	var mu sync.Mutex
	var seen []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.String()+" host="+r.Host)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(proxy.Close)
	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)

	resolver := &stubNetworkResolver{answers: map[string][]netip.Addr{
		"public.example.com":  {netip.MustParseAddr("8.8.8.8")},
		"public6.example.com": {netip.MustParseAddr("2606:4700:4700::1111")},
		"rebound.example.com": {netip.MustParseAddr("10.0.0.1")},
	}}
	client := &http.Client{
		Transport: newPublicNetworkTransport(resolver, func(transport *http.Transport) {
			transport.Proxy = http.ProxyURL(proxyURL)
		}),
		Timeout: 2 * time.Second,
	}

	for _, target := range []string{"http://public.example.com/hook", "http://public6.example.com:8080/hook"} {
		resp, err := client.Get(target)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, target, resp.Request.URL.String(), "the caller's request is reported back")
	}
	_, err = client.Get("http://rebound.example.com/hook")
	require.ErrorContains(t, err, "non-public address")

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{
		"http://8.8.8.8/hook host=8.8.8.8",
		"http://[2606:4700:4700::1111]:8080/hook host=[2606:4700:4700::1111]:8080",
	}, seen)
=======
func TestParseEgressPolicy(t *testing.T) {
	policy := parseEgressPolicy("", "", "10.0.0.0/8 ::ffff:172.16.0.0/108, 192.168.1.10, fe80::1%eth0, *.svc.cluster.local, Hooks.Example.COM., bad host, -, *.")
	assert.Equal(t, egressAllowAll, policy.level)
	assert.Equal(t, []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.1.10/32"),
	}, policy.allowedPrefixes)
	assert.Equal(t, []string{".svc.cluster.local", "hooks.example.com", "bad", "host"}, policy.allowedHosts)
>>>>>>> f1b4e3ccc (Fix webhook egress allow all (#24126))
}

// TestPublicNetworkTransportPinsProxiedHTTPSTarget checks the HTTPS tunnel: CONNECT goes to the
// validated address, and the certificate is still verified against the hostname.
func TestPublicNetworkTransportPinsProxiedHTTPSTarget(t *testing.T) {
	t.Setenv(privateNetworkAccessEnv, "false")
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(target.Close)

	var mu sync.Mutex
	var connects []string
	// the proxy records the CONNECT target and tunnels every request to the test server
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		mu.Lock()
		connects = append(connects, r.Host)
		mu.Unlock()
		upstream, err := net.Dial("tcp", target.Listener.Addr().String())
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		conn, buffered, err := http.NewResponseController(w).Hijack()
		if err != nil {
			_ = upstream.Close()
			return
		}
		go func() {
			_, _ = buffered.WriteTo(upstream)
			_, _ = io.Copy(upstream, conn)
			_ = upstream.Close()
		}()
		_, _ = io.Copy(conn, upstream)
		_ = conn.Close()
	}))
	t.Cleanup(proxy.Close)
	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)

	resolver := &stubNetworkResolver{answers: map[string][]netip.Addr{
		// httptest's certificate is issued for example.com
		"example.com": {netip.MustParseAddr("8.8.8.8")},
		"other.test":  {netip.MustParseAddr("8.8.4.4")},
	}}
	roots := target.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	client := &http.Client{
		Transport: newPublicNetworkTransport(resolver, func(transport *http.Transport) {
			transport.Proxy = http.ProxyURL(proxyURL)
			transport.TLSClientConfig = &tls.Config{RootCAs: roots}
		}),
		Timeout: 2 * time.Second,
	}

	resp, err := client.Get("https://example.com/hook")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// a hostname the certificate does not cover still fails verification
	_, err = client.Get("https://other.test/hook")
	require.Error(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"8.8.8.8:443", "8.8.4.4:443"}, connects)
}
