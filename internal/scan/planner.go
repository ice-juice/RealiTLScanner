package scan

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"net"
	"net/netip"
	"sort"
	"strings"
	"time"
)

// BGPResolver resolves BGP prefixes for a seed IP.
type BGPResolver interface {
	PrefixesForIP(context.Context, netip.Addr, TargetScope) ([]netip.Prefix, error)
}

// TargetPlanOptions controls single-address target expansion.
type TargetPlanOptions struct {
	Scope          TargetScope
	Limit          int
	Resolver       BGPResolver
	BGPTimeout     time.Duration
	EnableIPv6     bool
	ASNMaxPrefixes int
	MaxSeenTargets int
	Logger         *slog.Logger
}

type addrSeenSet struct {
	m      map[netip.Addr]struct{}
	max    int
	warned bool
	log    *slog.Logger
}

func newAddrSeenSet(max int, log *slog.Logger) *addrSeenSet {
	return &addrSeenSet{
		m:   make(map[netip.Addr]struct{}),
		max: max,
		log: loggerOrDiscard(log),
	}
}

// remember returns true if addr was already recorded. When the table is full,
// new addresses are treated as unseen (may be scanned again) and are not stored.
func (s *addrSeenSet) remember(addr netip.Addr) bool {
	if _, exists := s.m[addr]; exists {
		return true
	}
	if s.max > 0 && len(s.m) >= s.max {
		if !s.warned {
			s.warned = true
			s.log.Warn("target dedupe table reached capacity, later targets may be scanned more than once", "max", s.max)
		}
		return false
	}
	s.m[addr] = struct{}{}
	return false
}

// NormalizeTargetPlanOptions fills zero values. Limit 0 stays unlimited.
func NormalizeTargetPlanOptions(opts TargetPlanOptions) TargetPlanOptions {
	if opts.Scope == "" {
		opts.Scope = TargetScopePrefix
	}
	if opts.Limit < 0 {
		opts.Limit = DefaultTargetLimit
	}
	if opts.BGPTimeout <= 0 {
		opts.BGPTimeout = 5 * time.Second
	}
	if opts.ASNMaxPrefixes <= 0 {
		opts.ASNMaxPrefixes = 32
	}
	if opts.Resolver == nil && opts.Scope != TargetScopeNearby {
		opts.Resolver = &RIPEStatResolver{
			IncludeIPv6:    opts.EnableIPv6,
			ASNMaxPrefixes: opts.ASNMaxPrefixes,
			Logger:         opts.Logger,
		}
	}
	opts.Logger = loggerOrDiscard(opts.Logger)
	return opts
}

// IterateAddrWithOptions expands a single address, CIDR, or domain into hosts.
func IterateAddrWithOptions(ctx context.Context, addr string, opts TargetPlanOptions) <-chan Host {
	hostChan := make(chan Host)
	opts = NormalizeTargetPlanOptions(opts)
	log := opts.Logger

	if _, _, err := net.ParseCIDR(addr); err == nil {
		return Iterate(ctx, strings.NewReader(addr), opts.EnableIPv6, log)
	}

	seed, ok := parseOrLookupAddr(ctx, addr, opts.EnableIPv6)
	if !ok {
		close(hostChan)
		log.Error("Not a valid IP, IP CIDR or domain", "addr", addr)
		return hostChan
	}

	go func() {
		defer close(hostChan)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		if opts.Scope == TargetScopeNearby {
			for host := range IterateNearbyHosts(ctx, seed, addr, opts.Limit, opts.MaxSeenTargets, log) {
				if !sendHost(ctx, hostChan, host) {
					return
				}
			}
			return
		}

		prefixes, err := resolveBGPPrefixes(ctx, seed, opts)
		if err != nil || len(prefixes) == 0 {
			if err != nil {
				log.Warn("BGP prefix lookup failed, falling back to nearby scan", "ip", seed.String(), "err", err)
			} else {
				log.Warn("BGP prefix lookup returned no prefixes, falling back to nearby scan", "ip", seed.String())
			}
			for host := range IterateNearbyHosts(ctx, seed, addr, opts.Limit, opts.MaxSeenTargets, log) {
				if !sendHost(ctx, hostChan, host) {
					return
				}
			}
			return
		}

		log.Info("Using BGP target scope", "scope", opts.Scope, "prefixes", len(prefixes), "limit", opts.Limit)
		for host := range IteratePrefixHosts(ctx, prefixes, seed, addr, opts.Limit, opts.MaxSeenTargets, log) {
			if !sendHost(ctx, hostChan, host) {
				return
			}
		}
	}()

	return hostChan
}

// IterateNearbyHosts walks numerically adjacent IPs around seed.
func IterateNearbyHosts(ctx context.Context, seed netip.Addr, origin string, limit int, maxSeen int, log *slog.Logger) <-chan Host {
	if limit < 0 {
		limit = DefaultTargetLimit
	}
	start, end := addrSpaceBounds(seed)
	return iterateBoundedHosts(ctx, seed, origin, start, end, limit, maxSeen, log)
}

// IteratePrefixHosts emits hosts from prefixes, then nearby IPs when limit is 0.
func IteratePrefixHosts(ctx context.Context, prefixes []netip.Prefix, seed netip.Addr, origin string, limit int, maxSeen int, log *slog.Logger) <-chan Host {
	hostChan := make(chan Host)
	if limit < 0 {
		limit = DefaultTargetLimit
	}
	log = loggerOrDiscard(log)

	go func() {
		defer close(hostChan)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		remaining := limit
		seen := newAddrSeenSet(maxSeen, log)
		for _, prefix := range orderPrefixes(prefixes, seed) {
			if ctx.Err() != nil {
				return
			}
			if limit > 0 && remaining <= 0 {
				return
			}
			if prefix.Addr().Is4() != seed.Is4() {
				continue
			}
			start, end, ok := prefixBounds(prefix)
			if !ok {
				continue
			}
			for host := range iterateBoundedHostsWithSeen(ctx, prefix, seed, origin, start, end, remaining, seen) {
				if !sendHost(ctx, hostChan, host) {
					return
				}
				if limit > 0 {
					remaining--
					if remaining <= 0 {
						return
					}
				}
			}
		}
		if limit == 0 {
			start, end := addrSpaceBounds(seed)
			for host := range iterateBoundedHostsWithSeen(ctx, netip.Prefix{}, seed, origin, start, end, 0, seen) {
				if !sendHost(ctx, hostChan, host) {
					return
				}
			}
		}
	}()

	return hostChan
}

func resolveBGPPrefixes(ctx context.Context, seed netip.Addr, opts TargetPlanOptions) ([]netip.Prefix, error) {
	if opts.Resolver == nil {
		return nil, errors.New("missing BGP resolver")
	}
	lookupCtx, cancel := context.WithTimeout(ctx, opts.BGPTimeout)
	defer cancel()
	prefixes, err := opts.Resolver.PrefixesForIP(lookupCtx, seed, opts.Scope)
	if err != nil {
		return nil, err
	}
	return filterPrefixes(prefixes, opts.EnableIPv6), nil
}

func parseOrLookupAddr(ctx context.Context, addr string, allowIPv6 bool) (netip.Addr, bool) {
	if ip, err := netip.ParseAddr(addr); err == nil {
		ip = ip.Unmap()
		if ip.Is4() || allowIPv6 {
			return ip, true
		}
		return netip.Addr{}, false
	}
	ip, err := LookupIP(ctx, addr, allowIPv6)
	if err != nil {
		return netip.Addr{}, false
	}
	parsed, err := netip.ParseAddr(ip.String())
	if err != nil {
		return netip.Addr{}, false
	}
	return parsed.Unmap(), true
}

func filterPrefixes(prefixes []netip.Prefix, allowIPv6 bool) []netip.Prefix {
	seen := make(map[string]struct{})
	filtered := make([]netip.Prefix, 0, len(prefixes))
	for _, prefix := range prefixes {
		prefix = prefix.Masked()
		if !prefix.IsValid() {
			continue
		}
		if !prefix.Addr().Is4() && !allowIPv6 {
			continue
		}
		key := prefix.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, prefix)
	}
	return filtered
}

func orderPrefixes(prefixes []netip.Prefix, seed netip.Addr) []netip.Prefix {
	ordered := append([]netip.Prefix(nil), prefixes...)
	sort.SliceStable(ordered, func(i, j int) bool {
		iContains := ordered[i].Contains(seed)
		jContains := ordered[j].Contains(seed)
		if iContains != jContains {
			return iContains
		}
		return ordered[i].Bits() > ordered[j].Bits()
	})
	return ordered
}

func iterateBoundedHosts(ctx context.Context, seed netip.Addr, origin string, start, end *big.Int, limit int, maxSeen int, log *slog.Logger) <-chan Host {
	return iterateBoundedHostsWithSeen(ctx, netip.Prefix{}, seed, origin, start, end, limit, newAddrSeenSet(maxSeen, log))
}

func iterateBoundedHostsWithSeen(ctx context.Context, prefix netip.Prefix, seed netip.Addr, origin string, start, end *big.Int, limit int, seen *addrSeenSet) <-chan Host {
	hostChan := make(chan Host)
	go func() {
		defer close(hostChan)
		if limit < 0 {
			return
		}
		seedValue, size, ok := addrToBig(seed)
		if !ok {
			return
		}
		var count int64
		emit := func(value *big.Int) bool {
			if value.Cmp(start) < 0 || value.Cmp(end) > 0 || limitReached(count, limit) {
				return false
			}
			addr, ok := bigToAddr(value, size)
			if !ok {
				return false
			}
			if prefix.IsValid() && !prefix.Contains(addr) {
				return false
			}
			if seen.remember(addr) {
				return false
			}
			originValue := addr.String()
			if addr == seed {
				originValue = origin
			}
			if !sendHost(ctx, hostChan, Host{
				IP:     netIPFromAddr(addr),
				Origin: originValue,
				Type:   HostTypeIP,
			}) {
				return false
			}
			count++
			return true
		}

		if seedValue.Cmp(start) >= 0 && seedValue.Cmp(end) <= 0 {
			emit(seedValue)
			for offset := big.NewInt(1); !limitReached(count, limit); offset.Add(offset, big.NewInt(1)) {
				if ctx.Err() != nil {
					return
				}
				lower := new(big.Int).Sub(seedValue, offset)
				upper := new(big.Int).Add(seedValue, offset)
				emitted := false
				if lower.Cmp(start) >= 0 {
					emitted = emit(lower) || emitted
				}
				if limitReached(count, limit) || ctx.Err() != nil {
					return
				}
				if upper.Cmp(end) <= 0 {
					emitted = emit(upper) || emitted
				}
				if !emitted && lower.Cmp(start) < 0 && upper.Cmp(end) > 0 {
					return
				}
			}
			return
		}

		for value := new(big.Int).Set(start); !limitReached(count, limit) && value.Cmp(end) <= 0; value.Add(value, big.NewInt(1)) {
			if ctx.Err() != nil {
				return
			}
			emit(value)
		}
	}()
	return hostChan
}

func addrSpaceBounds(seed netip.Addr) (*big.Int, *big.Int) {
	bits := seed.BitLen()
	start := big.NewInt(0)
	end := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), uint(bits)), big.NewInt(1))
	return start, end
}

func limitReached(count int64, limit int) bool {
	return limit > 0 && count >= int64(limit)
}

func prefixBounds(prefix netip.Prefix) (*big.Int, *big.Int, bool) {
	prefix = prefix.Masked()
	start, _, ok := addrToBig(prefix.Addr())
	if !ok {
		return nil, nil, false
	}
	hostBits := prefix.Addr().BitLen() - prefix.Bits()
	count := new(big.Int).Lsh(big.NewInt(1), uint(hostBits))
	end := new(big.Int).Add(start, new(big.Int).Sub(count, big.NewInt(1)))
	return start, end, true
}

func addrToBig(addr netip.Addr) (*big.Int, int, bool) {
	addr = addr.Unmap()
	if addr.Is4() {
		raw := addr.As4()
		return new(big.Int).SetBytes(raw[:]), 4, true
	}
	if addr.Is6() {
		raw := addr.As16()
		return new(big.Int).SetBytes(raw[:]), 16, true
	}
	return nil, 0, false
}

func bigToAddr(value *big.Int, size int) (netip.Addr, bool) {
	if value.Sign() < 0 || len(value.Bytes()) > size {
		return netip.Addr{}, false
	}
	raw := value.Bytes()
	padded := append(make([]byte, size-len(raw)), raw...)
	switch size {
	case 4:
		var arr [4]byte
		copy(arr[:], padded)
		return netip.AddrFrom4(arr), true
	case 16:
		var arr [16]byte
		copy(arr[:], padded)
		return netip.AddrFrom16(arr), true
	default:
		return netip.Addr{}, false
	}
}

func netIPFromAddr(addr netip.Addr) net.IP {
	addr = addr.Unmap()
	if addr.Is4() {
		raw := addr.As4()
		return net.IPv4(raw[0], raw[1], raw[2], raw[3])
	}
	raw := addr.As16()
	ip := make(net.IP, len(raw))
	copy(ip, raw[:])
	return ip
}
