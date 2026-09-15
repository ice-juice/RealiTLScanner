package scan

import (
	"net"
	"testing"
)

func TestGeoWithoutDatabaseReturnsNA(t *testing.T) {
	geo := &Geo{}

	if got := geo.GetGeo(net.ParseIP("192.0.2.1")); got != "N/A" {
		t.Fatalf("expected N/A without GeoIP database, got %q", got)
	}
}

func TestNewGeoEmptyPathDoesNotError(t *testing.T) {
	geo := NewGeo("", nil)
	defer geo.Close()
	if got := geo.GetGeo(net.ParseIP("192.0.2.1")); got != "N/A" {
		t.Fatalf("expected N/A for empty path, got %q", got)
	}
}
