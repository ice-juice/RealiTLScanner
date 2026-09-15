package scan

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strconv"
	"time"
)

// ScanPipelineOptions configures the probe → TLS scan workers.
type ScanPipelineOptions struct {
	Threads      int
	ProbeThreads int
	ScanThreads  int
	ProbeTimeout time.Duration
	EnableIPv6   bool
	Logger       *slog.Logger
	Probe        func(context.Context, Host) bool
	Scan         func(context.Context, Host)
}

// NormalizeScanPipelineOptions fills zero thread counts and probe timeout.
func NormalizeScanPipelineOptions(opts ScanPipelineOptions) ScanPipelineOptions {
	if opts.ProbeThreads <= 0 {
		if opts.Threads > 0 {
			opts.ProbeThreads = opts.Threads
		} else {
			opts.ProbeThreads = 1
		}
	}
	if opts.ScanThreads <= 0 {
		if opts.Threads > 0 {
			opts.ScanThreads = opts.Threads
		} else {
			opts.ScanThreads = 1
		}
	}
	if opts.Threads <= 0 {
		opts.Threads = opts.ScanThreads
	}
	if opts.ProbeTimeout <= 0 {
		opts.ProbeTimeout = time.Second
	}
	return opts
}

// RunScanPipeline runs optional TCP probes then TLS scans until hosts close or ctx is cancelled.
func RunScanPipeline(ctx context.Context, hosts <-chan Host, opts ScanPipelineOptions) error {
	opts = NormalizeScanPipelineOptions(opts)
	if opts.Scan == nil {
		return errors.New("missing TLS scan function")
	}

	scanInput := hosts
	if opts.Probe != nil {
		probed := make(chan Host)
		done := make(chan struct{})
		for i := 0; i < opts.ProbeThreads; i++ {
			go func() {
				defer func() { done <- struct{}{} }()
				for {
					select {
					case <-ctx.Done():
						return
					case host, ok := <-hosts:
						if !ok {
							return
						}
						if opts.Probe(ctx, host) {
							select {
							case probed <- host:
							case <-ctx.Done():
								return
							}
						}
					}
				}
			}()
		}
		go func() {
			for i := 0; i < opts.ProbeThreads; i++ {
				<-done
			}
			close(probed)
		}()
		scanInput = probed
	}

	done := make(chan struct{})
	for i := 0; i < opts.ScanThreads; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for {
				select {
				case <-ctx.Done():
					return
				case host, ok := <-scanInput:
					if !ok {
						return
					}
					opts.Scan(ctx, host)
				}
			}
		}()
	}
	for i := 0; i < opts.ScanThreads; i++ {
		select {
		case <-done:
		case <-ctx.Done():
			// Wait for workers to observe cancel and exit so we do not leak them.
			for remaining := opts.ScanThreads - i; remaining > 0; remaining-- {
				<-done
			}
			return ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// ProbeTCP checks whether host:port accepts a TCP connection.
func ProbeTCP(ctx context.Context, host Host, port int, probeTimeout time.Duration, enableIPv6 bool, log *slog.Logger) bool {
	log = loggerOrDiscard(log)
	if probeTimeout <= 0 {
		probeTimeout = time.Second
	}
	if host.IP == nil {
		ip, err := LookupIP(ctx, host.Origin, enableIPv6)
		if err != nil {
			log.Debug("TCP probe failed to resolve host", "origin", host.Origin, "err", err)
			return false
		}
		host.IP = ip
	}
	hostPort := net.JoinHostPort(host.IP.String(), strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: probeTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", hostPort)
	if err != nil {
		log.Debug("TCP probe failed", "target", hostPort)
		return false
	}
	_ = conn.Close()
	log.Debug("TCP probe succeeded", "target", hostPort)
	return true
}
