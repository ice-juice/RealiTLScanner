package scan

import (
	"bytes"
	"context"
	"encoding/csv"
	"net"
	"strings"
	"testing"
	"time"
)

func TestCSVResultWriterEscapesCommaContainingIssuer(t *testing.T) {
	var buf bytes.Buffer
	writer := NewCSVResultWriter(&buf)

	writer.Write(csvHeader)
	writer.Write([]string{
		"154.36.179.29",
		"154.36.179.29",
		"TLS 1.3",
		"h2",
		"X25519",
		"919(certs count: 1)",
		"SHA256-RSA",
		"RSA",
		"cdn.example.com",
		"CDN.Data.Center,CDN Data Center",
		"N/A",
	})
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	records, err := csv.NewReader(bytes.NewReader(buf.Bytes())).ReadAll()
	if err != nil {
		t.Fatalf("read generated csv: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected header and one record, got %d records", len(records))
	}
	if len(records[1]) != len(csvHeader) {
		t.Fatalf("expected %d columns, got %d: %#v", len(csvHeader), len(records[1]), records[1])
	}
	if records[1][9] != "CDN.Data.Center,CDN Data Center" {
		t.Fatalf("issuer was not preserved, got %q", records[1][9])
	}
	if records[1][10] != "N/A" {
		t.Fatalf("expected GEO_CODE to stay in the last column, got %q", records[1][10])
	}
}

func TestCSVResultWriterFlushesRowsBeforeClose(t *testing.T) {
	var buf bytes.Buffer
	writer := NewCSVResultWriter(&buf)
	defer writer.Close()

	writer.Write(csvHeader)
	writer.Write([]string{
		"154.29.156.15",
		"154.29.156.15",
		"TLS 1.3",
		"h2",
		"X25519",
		"1000(certs count: 1)",
		"SHA256-RSA",
		"RSA",
		"example.com",
		"Example CA",
		"N/A",
	})

	deadline := time.Now().Add(time.Second)
	for {
		records, err := csv.NewReader(bytes.NewReader(buf.Bytes())).ReadAll()
		if err == nil && len(records) == 2 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected rows to be flushed before close, current bytes: %q", buf.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestIterateParsesIPCIDRAndDomain(t *testing.T) {
	hosts := collectHosts(Iterate(context.Background(), strings.NewReader("192.0.2.1\n192.0.2.8/30\nexample.com\nnot valid!\n"), false, nil))

	if len(hosts) != 6 {
		t.Fatalf("expected 6 parsed hosts, got %d: %#v", len(hosts), hosts)
	}
	if hosts[0].Type != HostTypeIP || hosts[0].IP.String() != "192.0.2.1" {
		t.Fatalf("expected first host to be IPv4, got %#v", hosts[0])
	}
	if hosts[1].Type != HostTypeCIDR || hosts[1].IP.String() != "192.0.2.8" {
		t.Fatalf("expected CIDR expansion to start at 192.0.2.8, got %#v", hosts[1])
	}
	if hosts[5].Type != HostTypeDomain || hosts[5].Origin != "example.com" {
		t.Fatalf("expected final host to be example.com domain, got %#v", hosts[5])
	}
}

func TestUtilityHelpers(t *testing.T) {
	if !ValidateDomainName("sub.example.com") {
		t.Fatal("expected simple domain to be valid")
	}
	if ValidateDomainName("bad domain") {
		t.Fatal("expected domain with spaces to be invalid")
	}
	if !ExistOnlyOne([]string{"", "addr", ""}) {
		t.Fatal("expected exactly one populated value")
	}
	if ExistOnlyOne([]string{"addr", "in", ""}) {
		t.Fatal("expected multiple populated values to be rejected")
	}

	deduped := RemoveDuplicateStr([]string{"a", "b", "a"})
	if len(deduped) != 2 || deduped[0] != "a" || deduped[1] != "b" {
		t.Fatalf("unexpected dedupe result: %#v", deduped)
	}

	next := NextIP(net.ParseIP("192.0.2.1"), true)
	if next.String() != "192.0.2.2" {
		t.Fatalf("expected next IP 192.0.2.2, got %s", next.String())
	}
	prev := NextIP(net.ParseIP("192.0.2.1"), false)
	if prev.String() != "192.0.2.0" {
		t.Fatalf("expected previous IP 192.0.2.0, got %s", prev.String())
	}
}
