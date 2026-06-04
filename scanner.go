package main

import (
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

func ScanTLS(host Host, out chan<- []string, geo *Geo, domains *DomainDeduper) {
	if host.IP == nil {
		ip, err := LookupIP(host.Origin)
		if err != nil {
			slog.Debug("Failed to get IP from the origin", "origin", host.Origin, "err", err)
			return
		}
		host.IP = ip
	}
	hostPort := net.JoinHostPort(host.IP.String(), strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", hostPort, time.Duration(timeout)*time.Second)
	if err != nil {
		slog.Debug("Cannot dial", "target", hostPort)
		return
	}
	defer conn.Close()
	err = conn.SetDeadline(time.Now().Add(time.Duration(timeout) * time.Second))
	if err != nil {
		slog.Error("Error setting deadline", "err", err)
		return
	}
	tlsCfg := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2", "http/1.1"},
		CurvePreferences:   []tls.CurveID{tls.X25519, tls.X25519MLKEM768},
	}
	if host.Type == HostTypeDomain {
		tlsCfg.ServerName = host.Origin
	}
	c := tls.Client(conn, tlsCfg)
	err = c.Handshake()
	if err != nil {
		slog.Debug("TLS handshake failed", "target", hostPort)
		return
	}
	state := c.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		slog.Debug("TLS handshake returned no peer certificates", "target", hostPort)
		return
	}
	alpn := state.NegotiatedProtocol
	leaf := state.PeerCertificates[0]
	domain, hasDomain := selectCertificateDomain(leaf)
	issuers := strings.Join(leaf.Issuer.Organization, " | ")
	length := 0
	for _, cert := range state.PeerCertificates {
		length += len(cert.Raw)
	}

	log := slog.Info
	feasible := state.Version == tls.VersionTLS13 && alpn == "h2" && hasDomain && len(issuers) != 0
	duplicateDomain := false
	geoCode := geo.GetGeo(host.IP)
	if feasible && domains != nil {
		duplicateDomain = !domains.Add(domain)
		feasible = !duplicateDomain
	}
	if !feasible {
		// not feasible
		log = slog.Debug
	} else {
		out <- []string{
			host.IP.String(),
			host.Origin,
			tls.VersionName(state.Version),
			alpn,
			state.CurveID.String(),
			strconv.Itoa(length) + "(certs count: " + strconv.Itoa(len(state.PeerCertificates)) + ")",
			leaf.SignatureAlgorithm.String(),
			leaf.PublicKeyAlgorithm.String(),
			domain,
			issuers,
			geoCode,
		}
	}
	log("Connected to target", "feasible", feasible,
		"ip", host.IP.String(),
		"origin", host.Origin,
		"tls", tls.VersionName(state.Version),
		"alpn", alpn,
		"curve", state.CurveID.String(),
		"cert-length", strconv.Itoa(length)+"(certs count: "+strconv.Itoa(len(state.PeerCertificates))+")",
		"cert-signature", leaf.SignatureAlgorithm.String(),
		"cert-publickey", leaf.PublicKeyAlgorithm.String(),
		"cert-domain", domain,
		"cert-issuer", issuers,
		"duplicate-domain", duplicateDomain,
		"geo", geoCode)
}

type DomainDeduper struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func NewDomainDeduper() *DomainDeduper {
	return &DomainDeduper{
		seen: make(map[string]struct{}),
	}
}

func (o *DomainDeduper) Add(domain string) bool {
	domain, ok := normalizeCertificateDomain(domain)
	if !ok {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, exists := o.seen[domain]; exists {
		return false
	}
	o.seen[domain] = struct{}{}
	return true
}

func selectCertificateDomain(cert *x509.Certificate) (string, bool) {
	for _, name := range cert.DNSNames {
		if domain, ok := normalizeCertificateDomain(name); ok {
			return domain, true
		}
	}
	return "", false
}

func normalizeCertificateDomain(domain string) (string, bool) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" || len(domain) > 253 {
		return "", false
	}

	wildcard := false
	if strings.HasPrefix(domain, "*.") {
		wildcard = true
		domain = strings.TrimPrefix(domain, "*.")
	} else if strings.Contains(domain, "*") {
		return "", false
	}
	if net.ParseIP(domain) != nil || strings.Contains(domain, ":") {
		return "", false
	}

	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", false
	}
	for _, label := range labels {
		if !isValidDNSLabel(label) {
			return "", false
		}
	}
	if isAllDigits(labels[len(labels)-1]) {
		return "", false
	}

	if wildcard {
		return "*." + domain, true
	}
	return domain, true
}

func isValidDNSLabel(label string) bool {
	if label == "" || len(label) > 63 {
		return false
	}
	if label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for i := 0; i < len(label); i++ {
		c := label[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			continue
		}
		return false
	}
	return true
}

func isAllDigits(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return value != ""
}
