package scan

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Iterate parses IPs, CIDRs, and domains from r and emits hosts until ctx is done.
func Iterate(ctx context.Context, reader io.Reader, enableIPv6 bool, log *slog.Logger) <-chan Host {
	log = loggerOrDiscard(log)
	scanner := bufio.NewScanner(reader)
	hostChan := make(chan Host)
	go func() {
		defer close(hostChan)
		for scanner.Scan() {
			if ctx.Err() != nil {
				return
			}
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			ip := net.ParseIP(line)
			if ip != nil && (ip.To4() != nil || enableIPv6) {
				if !sendHost(ctx, hostChan, Host{
					IP:     ip,
					Origin: line,
					Type:   HostTypeIP,
				}) {
					return
				}
				continue
			}
			_, _, err := net.ParseCIDR(line)
			if err == nil {
				p, err := netip.ParsePrefix(line)
				if err != nil {
					log.Warn("Invalid cidr", "cidr", line, "err", err)
					continue
				}
				if !p.Addr().Is4() && !enableIPv6 {
					continue
				}
				p = p.Masked()
				addr := p.Addr()
				for {
					if ctx.Err() != nil {
						return
					}
					if !p.Contains(addr) {
						break
					}
					ip = net.ParseIP(addr.String())
					if ip != nil {
						if !sendHost(ctx, hostChan, Host{
							IP:     ip,
							Origin: line,
							Type:   HostTypeCIDR,
						}) {
							return
						}
					}
					addr = addr.Next()
				}
				continue
			}
			if ValidateDomainName(line) {
				if !sendHost(ctx, hostChan, Host{
					IP:     nil,
					Origin: line,
					Type:   HostTypeDomain,
				}) {
					return
				}
				continue
			}
			log.Warn("Not a valid IP, IP CIDR or domain", "line", line)
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
			log.Error("Read file error", "err", err)
		}
	}()
	return hostChan
}

func sendHost(ctx context.Context, hostChan chan<- Host, host Host) bool {
	select {
	case hostChan <- host:
		return true
	case <-ctx.Done():
		return false
	}
}

// ValidateDomainName reports whether domain looks like a hostname.
func ValidateDomainName(domain string) bool {
	r := regexp.MustCompile(`(?m)^[A-Za-z0-9\-.]+$`)
	return r.MatchString(domain)
}

// ExistOnlyOne reports whether exactly one string in arr is non-empty.
func ExistOnlyOne(arr []string) bool {
	exist := false
	for _, item := range arr {
		if item != "" {
			if exist {
				return false
			}
			exist = true
		}
	}
	return exist
}

// LookupIP resolves addr, honoring enableIPv6 and ctx cancellation.
func LookupIP(ctx context.Context, addr string, enableIPv6 bool) (net.IP, error) {
	resolver := &net.Resolver{}
	ips, err := resolver.LookupIPAddr(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup: %w", err)
	}
	var arr []net.IP
	for _, ipa := range ips {
		ip := ipa.IP
		if ip.To4() != nil || enableIPv6 {
			arr = append(arr, ip)
		}
	}
	if len(arr) == 0 {
		return nil, errors.New("no IP found")
	}
	return arr[0], nil
}

// RemoveDuplicateStr returns strSlice with first-seen values only.
func RemoveDuplicateStr(strSlice []string) []string {
	allKeys := make(map[string]bool)
	var list []string
	for _, item := range strSlice {
		if _, value := allKeys[item]; !value {
			allKeys[item] = true
			list = append(list, item)
		}
	}
	return list
}

// NextIP returns the numerically adjacent IP.
func NextIP(ip net.IP, increment bool) net.IP {
	ipb := big.NewInt(0).SetBytes(ip)
	if increment {
		ipb.Add(ipb, big.NewInt(1))
	} else {
		ipb.Sub(ipb, big.NewInt(1))
	}
	b := ipb.Bytes()
	b = append(make([]byte, len(ip)-len(b)), b...)
	return b
}

// NewNoProxyHTTPClient returns an HTTP client that never uses environment proxies.
func NewNoProxyHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) { return nil, nil },
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}
