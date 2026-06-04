package main

import (
	"bytes"
	"encoding/csv"
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
