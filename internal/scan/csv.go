package scan

import (
	"encoding/csv"
	"io"
)

// CSVHeader is the stable 11-column output header. Order must not change.
var CSVHeader = []string{
	"IP",
	"ORIGIN",
	"TLS",
	"ALPN",
	"CURVE",
	"CERT_LENGTH",
	"CERT_SIGNATURE",
	"CERT_PUBLICKEY",
	"CERT_DOMAIN",
	"CERT_ISSUER",
	"GEO_CODE",
}

// csvHeader is an alias used by migrated tests.
var csvHeader = CSVHeader

// CSVResultWriter writes result rows and flushes after every write.
type CSVResultWriter struct {
	rows chan []string
	done chan error
}

// NewCSVResultWriter starts a background writer that flushes each row immediately.
func NewCSVResultWriter(writer io.Writer) *CSVResultWriter {
	rows := make(chan []string)
	done := make(chan error, 1)
	csvWriter := csv.NewWriter(writer)
	go func() {
		var firstErr error
		for row := range rows {
			if err := csvWriter.Write(row); err != nil && firstErr == nil {
				firstErr = err
			}
			csvWriter.Flush()
			if err := csvWriter.Error(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		csvWriter.Flush()
		if err := csvWriter.Error(); err != nil && firstErr == nil {
			firstErr = err
		}
		done <- firstErr
	}()
	return &CSVResultWriter{
		rows: rows,
		done: done,
	}
}

// Rows returns the inbound row channel.
func (o *CSVResultWriter) Rows() chan<- []string {
	return o.rows
}

// Write enqueues one CSV row.
func (o *CSVResultWriter) Write(row []string) {
	o.rows <- row
}

// Close stops the writer and returns the first write error, if any.
func (o *CSVResultWriter) Close() error {
	close(o.rows)
	return <-o.done
}
