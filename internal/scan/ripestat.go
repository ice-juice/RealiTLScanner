package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	neturl "net/url"
	"time"
)

// RIPEStatResolver looks up BGP prefixes via the RIPE Stat API.
type RIPEStatResolver struct {
	Client         *http.Client
	IncludeIPv6    bool
	ASNMaxPrefixes int
	Logger         *slog.Logger
}

// PrefixesForIP returns containing and optionally ASN-announced prefixes.
func (r *RIPEStatResolver) PrefixesForIP(ctx context.Context, ip netip.Addr, scope TargetScope) ([]netip.Prefix, error) {
	info, err := r.networkInfo(ctx, ip)
	if err != nil {
		return nil, err
	}

	var prefixes []netip.Prefix
	if info.Prefix.IsValid() {
		prefixes = append(prefixes, info.Prefix)
	}
	if scope != TargetScopeASN {
		return filterPrefixes(prefixes, r.IncludeIPv6), nil
	}
	if len(info.ASNs) == 0 {
		return filterPrefixes(prefixes, r.IncludeIPv6), nil
	}
	maxPrefixes := r.ASNMaxPrefixes
	if maxPrefixes <= 0 {
		maxPrefixes = 32
	}
	log := loggerOrDiscard(r.Logger)
	for _, asn := range info.ASNs {
		asnPrefixes, err := r.announcedPrefixes(ctx, asn)
		if err != nil {
			log.Warn("Cannot fetch ASN prefixes", "asn", asn, "err", err)
			continue
		}
		for _, prefix := range asnPrefixes {
			prefixes = append(prefixes, prefix)
			if len(prefixes) >= maxPrefixes {
				return filterPrefixes(prefixes, r.IncludeIPv6), nil
			}
		}
	}
	return filterPrefixes(prefixes, r.IncludeIPv6), nil
}

type ripeNetworkInfo struct {
	ASNs   []int
	Prefix netip.Prefix
}

func (r *RIPEStatResolver) networkInfo(ctx context.Context, ip netip.Addr) (ripeNetworkInfo, error) {
	var payload struct {
		Data struct {
			ASNs   []int  `json:"asns"`
			Prefix string `json:"prefix"`
		} `json:"data"`
	}
	if err := r.getJSON(ctx, "https://stat.ripe.net/data/network-info/data.json?resource="+neturl.QueryEscape(ip.String()), &payload); err != nil {
		return ripeNetworkInfo{}, err
	}
	var prefix netip.Prefix
	if payload.Data.Prefix != "" {
		parsed, err := netip.ParsePrefix(payload.Data.Prefix)
		if err == nil {
			prefix = parsed.Masked()
		}
	}
	return ripeNetworkInfo{ASNs: payload.Data.ASNs, Prefix: prefix}, nil
}

func (r *RIPEStatResolver) announcedPrefixes(ctx context.Context, asn int) ([]netip.Prefix, error) {
	var payload struct {
		Data struct {
			Prefixes []struct {
				Prefix string `json:"prefix"`
			} `json:"prefixes"`
		} `json:"data"`
	}
	endpoint := fmt.Sprintf("https://stat.ripe.net/data/announced-prefixes/data.json?resource=AS%d", asn)
	if err := r.getJSON(ctx, endpoint, &payload); err != nil {
		return nil, err
	}
	prefixes := make([]netip.Prefix, 0, len(payload.Data.Prefixes))
	for _, item := range payload.Data.Prefixes {
		prefix, err := netip.ParsePrefix(item.Prefix)
		if err != nil {
			continue
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func (r *RIPEStatResolver) getJSON(ctx context.Context, endpoint string, out any) error {
	client := r.Client
	if client == nil {
		client = NewNoProxyHTTPClient(30 * time.Second)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "RealiTLScanner/0.2")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("RIPE Stat returned %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return err
	}
	return nil
}
