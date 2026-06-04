package main

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"
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
	deduper := NewDomainDeduper()

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
