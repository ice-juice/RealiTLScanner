package scan

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestSelectCertificateDomainPrefersValidSANOverCommonName(t *testing.T) {
	cert := &x509.Certificate{
		Subject:  pkix.Name{CommonName: "CDN.Server"},
		DNSNames: []string{"WWW.Example.COM"},
	}

	domain, ok := selectCertificateDomain(cert)
	if !ok {
		t.Fatal("expected a valid SAN domain")
	}
	if domain != "www.example.com" {
		t.Fatalf("expected normalized SAN domain, got %q", domain)
	}
}

func TestSelectCertificateDomainRejectsCommonNameOnlyAndIPDomains(t *testing.T) {
	tests := []struct {
		name string
		cert *x509.Certificate
	}{
		{
			name: "common name only",
			cert: &x509.Certificate{Subject: pkix.Name{CommonName: "CDN.Server"}},
		},
		{
			name: "ip address in dns names",
			cert: &x509.Certificate{DNSNames: []string{"154.36.179.40"}},
		},
		{
			name: "single label dns name",
			cert: &x509.Certificate{DNSNames: []string{"localhost"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if domain, ok := selectCertificateDomain(tt.cert); ok {
				t.Fatalf("expected invalid certificate domain, got %q", domain)
			}
		})
	}
}

func TestDomainDeduperRejectsDuplicateDomains(t *testing.T) {
	deduper := NewDomainDeduper(0, nil)

	if !deduper.Add("WWW.Example.COM") {
		t.Fatal("expected first domain to be accepted")
	}
	if deduper.Add("www.example.com") {
		t.Fatal("expected duplicate domain to be rejected")
	}
	if !deduper.Add("api.example.com") {
		t.Fatal("expected different domain to be accepted")
	}
}

func TestDomainDeduperStopsGrowingAtCapacity(t *testing.T) {
	deduper := NewDomainDeduper(1, nil)
	if !deduper.Add("a.example.com") {
		t.Fatal("expected first domain to be stored")
	}
	if deduper.Len() != 1 {
		t.Fatalf("expected one stored domain, got %d", deduper.Len())
	}
	if !deduper.Add("b.example.com") {
		t.Fatal("expected overflow domain to still be treated as new")
	}
	if deduper.Len() != 1 {
		t.Fatalf("expected capacity to cap stored entries, got %d", deduper.Len())
	}
	if deduper.Add("a.example.com") {
		t.Fatal("expected already stored domain to stay a duplicate")
	}
}

func TestSNINameFromCertificateDomainRejectsWildcard(t *testing.T) {
	if name, ok := sniNameFromCertificateDomain("*.example.com"); ok {
		t.Fatalf("expected wildcard certificate domain to be rejected as direct SNI, got %q", name)
	}

	name, ok := sniNameFromCertificateDomain("WWW.Example.COM")
	if !ok {
		t.Fatal("expected ordinary certificate domain to be accepted as SNI")
	}
	if name != "www.example.com" {
		t.Fatalf("expected normalized SNI name, got %q", name)
	}
}

func TestCertificateContainsDomainMatchesNormalizedSAN(t *testing.T) {
	cert := &x509.Certificate{DNSNames: []string{"WWW.Example.COM", "*.example.net"}}

	if !certificateContainsDomain(cert, "www.example.com") {
		t.Fatal("expected certificate to contain normalized SAN")
	}
	if certificateContainsDomain(cert, "*.example.net") {
		t.Fatal("expected wildcard SAN to be rejected as a direct SNI name")
	}
}

func TestScanTLSWithOptionsWritesVerifiedSNIResult(t *testing.T) {
	listener, port := startLocalTLSServer(t, 2)
	defer listener.Close()

	host := Host{IP: net.ParseIP("127.0.0.1"), Origin: "127.0.0.1", Type: HostTypeIP}
	result, ok := ScanTLSWithOptions(context.Background(), host, &Geo{}, NewDomainDeduper(0, nil), TLSScanOptions{
		Port:      port,
		Timeout:   time.Second,
		VerifySNI: true,
	})
	if !ok {
		t.Fatal("expected TLS scan to write a feasible result")
	}
	row := result.CSVRow()
	if row[0] != "127.0.0.1" {
		t.Fatalf("expected localhost IP, got %q", row[0])
	}
	if row[2] != "TLS 1.3" {
		t.Fatalf("expected TLS 1.3, got %q", row[2])
	}
	if row[3] != "h2" {
		t.Fatalf("expected h2 ALPN, got %q", row[3])
	}
	if row[8] != "sni.example.com" {
		t.Fatalf("expected certificate domain, got %q", row[8])
	}
}

func startLocalTLSServer(t *testing.T, accepts int) (net.Listener, int) {
	t.Helper()
	cert := generateTestCertificate(t)
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		NextProtos:   []string{"h2"},
	})
	if err != nil {
		t.Fatalf("listen TLS: %v", err)
	}
	go func() {
		for i := 0; i < accepts; i++ {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			if tlsConn, ok := conn.(*tls.Conn); ok {
				_ = tlsConn.Handshake()
			}
			_ = conn.Close()
		}
	}()
	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split TLS listener address: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse TLS listener port: %v", err)
	}
	return listener, port
}

func generateTestCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Example Test CA"},
			CommonName:   "sni.example.com",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"sni.example.com"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}
}
