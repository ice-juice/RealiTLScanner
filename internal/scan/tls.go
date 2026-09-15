package scan

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"
)

// TLSScanOptions configures a single TLS handshake attempt.
type TLSScanOptions struct {
	Port       int
	Timeout    time.Duration
	VerifySNI  bool
	EnableIPv6 bool
	Logger     *slog.Logger
}

// NormalizeTLSScanOptions fills port/timeout from library defaults, not globals.
func NormalizeTLSScanOptions(opts TLSScanOptions) TLSScanOptions {
	if opts.Port <= 0 {
		opts.Port = 443
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	opts.Logger = loggerOrDiscard(opts.Logger)
	return opts
}

// ScanTLSWithOptions performs the TLS handshake and Reality-feasibility checks.
// ok is true only when the target is written as a hit.
func ScanTLSWithOptions(ctx context.Context, host Host, geo *Geo, domains *DomainDeduper, opts TLSScanOptions) (Result, bool) {
	opts = NormalizeTLSScanOptions(opts)
	log := opts.Logger
	if host.IP == nil {
		ip, err := LookupIP(ctx, host.Origin, opts.EnableIPv6)
		if err != nil {
			log.Debug("Failed to get IP from the origin", "origin", host.Origin, "err", err)
			return Result{}, false
		}
		host.IP = ip
	}
	serverName := ""
	if host.Type == HostTypeDomain {
		serverName = host.Origin
	}
	state, err := handshakeTLS(ctx, host.IP, opts, serverName)
	if err != nil {
		log.Debug("TLS handshake failed", "target", net.JoinHostPort(host.IP.String(), strconv.Itoa(opts.Port)), "err", err)
		return Result{}, false
	}
	if len(state.PeerCertificates) == 0 {
		log.Debug("TLS handshake returned no peer certificates", "target", net.JoinHostPort(host.IP.String(), strconv.Itoa(opts.Port)))
		return Result{}, false
	}
	alpn := state.NegotiatedProtocol
	leaf := state.PeerCertificates[0]
	domain, hasDomain := selectCertificateDomain(leaf)
	issuers := strings.Join(leaf.Issuer.Organization, " | ")
	length := 0
	for _, cert := range state.PeerCertificates {
		length += len(cert.Raw)
	}

	logFn := log.Info
	feasible := state.Version == tls.VersionTLS13 && alpn == "h2" && hasDomain && len(issuers) != 0
	duplicateDomain := false
	sniVerified := !opts.VerifySNI || host.Type == HostTypeDomain
	if feasible && opts.VerifySNI && host.Type != HostTypeDomain {
		sniName, ok := sniNameFromCertificateDomain(domain)
		if ok {
			sniVerified = verifySNI(ctx, host.IP, sniName, opts)
		}
		feasible = sniVerified
	}
	geoCode := "N/A"
	if geo != nil {
		geoCode = geo.GetGeo(host.IP)
	}
	if feasible && domains != nil {
		duplicateDomain = !domains.Add(domain)
		feasible = !duplicateDomain
	}
	if !feasible {
		logFn = log.Debug
	}
	result := Result{
		IP:            host.IP.String(),
		Origin:        host.Origin,
		TLS:           tls.VersionName(state.Version),
		ALPN:          alpn,
		Curve:         state.CurveID.String(),
		CertLength:    strconv.Itoa(length) + "(certs count: " + strconv.Itoa(len(state.PeerCertificates)) + ")",
		CertSignature: leaf.SignatureAlgorithm.String(),
		CertPublicKey: leaf.PublicKeyAlgorithm.String(),
		CertDomain:    domain,
		CertIssuer:    issuers,
		GeoCode:       geoCode,
	}
	logFn("Connected to target", "feasible", feasible,
		"ip", host.IP.String(),
		"origin", host.Origin,
		"tls", result.TLS,
		"alpn", alpn,
		"curve", result.Curve,
		"cert-length", result.CertLength,
		"cert-signature", result.CertSignature,
		"cert-publickey", result.CertPublicKey,
		"cert-domain", domain,
		"cert-issuer", issuers,
		"duplicate-domain", duplicateDomain,
		"sni-verified", sniVerified,
		"geo", geoCode)
	if !feasible {
		return Result{}, false
	}
	return result, true
}

func handshakeTLS(ctx context.Context, ip net.IP, opts TLSScanOptions, serverName string) (tls.ConnectionState, error) {
	hostPort := net.JoinHostPort(ip.String(), strconv.Itoa(opts.Port))
	dialer := &net.Dialer{Timeout: opts.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", hostPort)
	if err != nil {
		return tls.ConnectionState{}, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(opts.Timeout)); err != nil {
		return tls.ConnectionState{}, err
	}
	tlsCfg := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2", "http/1.1"},
		CurvePreferences:   []tls.CurveID{tls.X25519, tls.X25519MLKEM768},
		ServerName:         serverName,
	}
	c := tls.Client(conn, tlsCfg)
	if err := c.HandshakeContext(ctx); err != nil {
		return tls.ConnectionState{}, err
	}
	return c.ConnectionState(), nil
}

func verifySNI(ctx context.Context, ip net.IP, sniName string, opts TLSScanOptions) bool {
	state, err := handshakeTLS(ctx, ip, opts, sniName)
	if err != nil || len(state.PeerCertificates) == 0 {
		return false
	}
	leaf := state.PeerCertificates[0]
	issuers := strings.Join(leaf.Issuer.Organization, " | ")
	return state.Version == tls.VersionTLS13 &&
		state.NegotiatedProtocol == "h2" &&
		len(issuers) != 0 &&
		certificateContainsDomain(leaf, sniName)
}

func sniNameFromCertificateDomain(domain string) (string, bool) {
	domain, ok := normalizeCertificateDomain(domain)
	if !ok || strings.HasPrefix(domain, "*.") {
		return "", false
	}
	return domain, true
}

func certificateContainsDomain(cert *x509.Certificate, domain string) bool {
	domain, ok := sniNameFromCertificateDomain(domain)
	if !ok {
		return false
	}
	for _, name := range cert.DNSNames {
		normalized, ok := normalizeCertificateDomain(name)
		if !ok {
			continue
		}
		if normalized == domain {
			return true
		}
	}
	return false
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
