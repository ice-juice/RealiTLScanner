package scan

import (
	"log/slog"
	"net"
	"sync"

	"github.com/oschwald/geoip2-golang"
)

// Geo looks up ISO country codes from a MaxMind Country database.
type Geo struct {
	geoReader *geoip2.Reader
	mu        sync.Mutex
	log       *slog.Logger
}

// NewGeo opens path if non-empty. A missing or empty path degrades to "N/A".
func NewGeo(path string, log *slog.Logger) *Geo {
	log = loggerOrDiscard(log)
	geo := &Geo{
		mu:  sync.Mutex{},
		log: log,
	}
	if path == "" {
		return geo
	}
	reader, err := geoip2.Open(path)
	if err != nil {
		log.Warn("Cannot open Country.mmdb", "path", path, "err", err)
		return geo
	}
	log.Info("Enabled GeoIP", "path", path)
	geo.geoReader = reader
	return geo
}

// Close releases the database handle.
func (o *Geo) Close() {
	if o == nil || o.geoReader == nil {
		return
	}
	_ = o.geoReader.Close()
	o.geoReader = nil
}

// GetGeo returns the ISO country code or "N/A".
func (o *Geo) GetGeo(ip net.IP) string {
	if o == nil || o.geoReader == nil {
		return "N/A"
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	country, err := o.geoReader.Country(ip)
	if err != nil {
		loggerOrDiscard(o.log).Debug("Error reading geo", "err", err)
		return "N/A"
	}
	return country.Country.IsoCode
}
