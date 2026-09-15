package scan

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"
)

type stubBGPResolver struct {
	prefixes []netip.Prefix
	err      error
}

func (s stubBGPResolver) PrefixesForIP(context.Context, netip.Addr, TargetScope) ([]netip.Prefix, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.prefixes, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func collectHosts(ch <-chan Host) []Host {
	var hosts []Host
	for host := range ch {
		hosts = append(hosts, host)
	}
	return hosts
}

func hostIPs(hosts []Host) []string {
	ips := make([]string, 0, len(hosts))
	for _, host := range hosts {
		ips = append(ips, host.IP.String())
	}
	return ips
}

func takeHostIPs(t *testing.T, ch <-chan Host, count int) []string {
	t.Helper()
	ips := make([]string, 0, count)
	for len(ips) < count {
		select {
		case host, ok := <-ch:
			if !ok {
				t.Fatalf("host stream closed after %d hosts, expected %d", len(ips), count)
			}
			ips = append(ips, host.IP.String())
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for host %d", len(ips)+1)
		}
	}
	return ips
}

func TestIterateNearbyHostsAlternatesAroundSeedWithinLimit(t *testing.T) {
	seed := netip.MustParseAddr("192.0.2.10")

	hosts := collectHosts(IterateNearbyHosts(context.Background(), seed, "192.0.2.10", 5, 0, nil))
	got := hostIPs(hosts)
	want := []string{"192.0.2.10", "192.0.2.9", "192.0.2.11", "192.0.2.8", "192.0.2.12"}

	if len(got) != len(want) {
		t.Fatalf("expected %d hosts, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("host %d: expected %s, got %s", i, want[i], got[i])
		}
	}
}

func TestIteratePrefixHostsWithUnlimitedLimitContinuesNearbyAfterPrefix(t *testing.T) {
	seed := netip.MustParseAddr("192.0.2.10")
	prefixes := []netip.Prefix{netip.MustParsePrefix("192.0.2.10/32")}

	got := takeHostIPs(t, IteratePrefixHosts(context.Background(), prefixes, seed, "192.0.2.10", 0, 0, nil), 5)
	want := []string{"192.0.2.10", "192.0.2.9", "192.0.2.11", "192.0.2.8", "192.0.2.12"}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("host %d: expected %s, got %s", i, want[i], got[i])
		}
	}
}

func TestNormalizeTargetPlanOptionsKeepsZeroAsUnlimited(t *testing.T) {
	opts := NormalizeTargetPlanOptions(TargetPlanOptions{
		Scope: TargetScopeNearby,
		Limit: 0,
	})

	if opts.Limit != 0 {
		t.Fatalf("expected zero limit to mean unlimited, got %d", opts.Limit)
	}
}

func TestIterateAddrWithBGPUsesContainingPrefixAroundSeed(t *testing.T) {
	opts := TargetPlanOptions{
		Scope: TargetScopePrefix,
		Limit: 4,
		Resolver: stubBGPResolver{
			prefixes: []netip.Prefix{
				netip.MustParsePrefix("198.51.100.0/30"),
				netip.MustParsePrefix("192.0.2.8/29"),
			},
		},
	}

	hosts := collectHosts(IterateAddrWithOptions(context.Background(), "192.0.2.10", opts))
	got := hostIPs(hosts)
	want := []string{"192.0.2.10", "192.0.2.9", "192.0.2.11", "192.0.2.8"}

	if len(got) != len(want) {
		t.Fatalf("expected %d hosts, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("host %d: expected %s, got %s", i, want[i], got[i])
		}
	}
}

func TestIterateAddrWithBGPFallsBackToNearbyOnResolverError(t *testing.T) {
	opts := TargetPlanOptions{
		Scope:    TargetScopePrefix,
		Limit:    3,
		Resolver: stubBGPResolver{err: errors.New("bgp unavailable")},
	}

	hosts := collectHosts(IterateAddrWithOptions(context.Background(), "192.0.2.10", opts))
	got := hostIPs(hosts)
	want := []string{"192.0.2.10", "192.0.2.9", "192.0.2.11"}

	if len(got) != len(want) {
		t.Fatalf("expected %d hosts, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("host %d: expected %s, got %s", i, want[i], got[i])
		}
	}
}

func TestParseTargetScope(t *testing.T) {
	tests := []struct {
		value string
		want  TargetScope
		ok    bool
	}{
		{value: "", want: TargetScopePrefix, ok: true},
		{value: "nearby", want: TargetScopeNearby, ok: true},
		{value: "PREFIX", want: TargetScopePrefix, ok: true},
		{value: "asn", want: TargetScopeASN, ok: true},
		{value: "unknown", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, ok := ParseTargetScope(tt.value)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("expected (%q, %v), got (%q, %v)", tt.want, tt.ok, got, ok)
			}
		})
	}
}

func TestFilterPrefixesDedupesAndSkipsIPv6WhenDisabled(t *testing.T) {
	prefixes := []netip.Prefix{
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("2001:db8::/32"),
	}

	filtered := filterPrefixes(prefixes, false)
	if len(filtered) != 1 {
		t.Fatalf("expected one IPv4 prefix, got %#v", filtered)
	}
	if filtered[0].String() != "192.0.2.0/24" {
		t.Fatalf("expected IPv4 prefix to remain, got %s", filtered[0])
	}
}

func TestRIPEStatResolverFetchesContainingAndASNPrefixes(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var body string
			switch {
			case strings.Contains(req.URL.Path, "/network-info/"):
				body = `{"data":{"asns":[64500],"prefix":"192.0.2.0/24"}}`
			case strings.Contains(req.URL.Path, "/announced-prefixes/"):
				body = `{"data":{"prefixes":[{"prefix":"198.51.100.0/24"},{"prefix":"2001:db8::/32"}]}}`
			default:
				t.Fatalf("unexpected RIPE Stat path: %s", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}),
	}
	resolver := &RIPEStatResolver{Client: client, ASNMaxPrefixes: 4}

	prefixes, err := resolver.PrefixesForIP(context.Background(), netip.MustParseAddr("192.0.2.10"), TargetScopeASN)
	if err != nil {
		t.Fatalf("fetch prefixes: %v", err)
	}
	got := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		got = append(got, prefix.String())
	}
	want := []string{"192.0.2.0/24", "198.51.100.0/24"}
	if len(got) != len(want) {
		t.Fatalf("expected %d prefixes, got %#v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("prefix %d: expected %s, got %s", i, want[i], got[i])
		}
	}
}

func TestAddressConversionHelpersHandleIPv4AndIPv6(t *testing.T) {
	ipv4 := netip.MustParseAddr("192.0.2.10")
	value, size, ok := addrToBig(ipv4)
	if !ok || size != 4 {
		t.Fatalf("expected IPv4 conversion, got size=%d ok=%v", size, ok)
	}
	roundTripped, ok := bigToAddr(value, size)
	if !ok || roundTripped != ipv4 {
		t.Fatalf("expected IPv4 round trip, got %s ok=%v", roundTripped, ok)
	}

	ipv6 := netip.MustParseAddr("2001:db8::1")
	value, size, ok = addrToBig(ipv6)
	if !ok || size != 16 {
		t.Fatalf("expected IPv6 conversion, got size=%d ok=%v", size, ok)
	}
	roundTripped, ok = bigToAddr(value, size)
	if !ok || roundTripped != ipv6 {
		t.Fatalf("expected IPv6 round trip, got %s ok=%v", roundTripped, ok)
	}
}
