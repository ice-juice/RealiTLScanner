package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xtls/RealiTLScanner/internal/scan"
)

func main() {
	var (
		addr                  string
		in                    string
		port                  int
		thread                int
		out                   string
		timeout               int
		verbose               bool
		enableIPv6            bool
		url                   string
		scanScope             string
		maxTargets            int
		probeTimeout          int
		disablePortProbe      bool
		enableSNIVerification bool
		asnMaxPrefixes        int
	)

	flag.StringVar(&addr, "addr", "", "Specify an IP, IP CIDR or domain to scan")
	flag.StringVar(&in, "in", "", "Specify a file that contains multiple "+
		"IPs, IP CIDRs or domains to scan, divided by line break")
	flag.IntVar(&port, "port", 443, "Specify a HTTPS port to check")
	flag.IntVar(&thread, "thread", 2, "Count of concurrent tasks")
	flag.StringVar(&out, "out", "out.csv", "Output file to store the result")
	flag.IntVar(&timeout, "timeout", 10, "Timeout for every check")
	flag.BoolVar(&verbose, "v", false, "Verbose output")
	flag.BoolVar(&enableIPv6, "46", false, "Enable IPv6 in additional to IPv4")
	flag.StringVar(&url, "url", "", "Crawl the domain list from a URL, "+
		"e.g. https://launchpad.net/ubuntu/+archivemirrors")
	flag.StringVar(&scanScope, "scope", string(scan.TargetScopePrefix), "Target discovery scope for single addr: nearby, prefix, or asn")
	flag.IntVar(&maxTargets, "limit", scan.DefaultTargetLimit, "Maximum generated targets for single addr BGP/nearby scanning; 0 means continuous")
	flag.IntVar(&probeTimeout, "probe-timeout", 1, "TCP port prefilter timeout in seconds")
	flag.BoolVar(&disablePortProbe, "no-port-probe", false, "Disable TCP port prefilter before TLS scanning")
	flag.BoolVar(&enableSNIVerification, "verify-sni", true, "Verify extracted certificate domain as SNI before output")
	flag.IntVar(&asnMaxPrefixes, "asn-prefixes", 32, "Maximum ASN prefixes to fetch when -scope asn is used")
	flag.Parse()

	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	if !scan.ExistOnlyOne([]string{addr, in, url}) {
		logger.Error("You must specify and only specify one of `addr`, `in`, or `url`")
		flag.PrintDefaults()
		return
	}
	scope, ok := scan.ParseTargetScope(scanScope)
	if !ok {
		logger.Error("Invalid scope, expected nearby, prefix, or asn", "scope", scanScope)
		flag.PrintDefaults()
		return
	}

	cfg := scan.Config{
		Port:             port,
		ScanThreads:      thread,
		ProbeThreads:     thread,
		Timeout:          time.Duration(timeout) * time.Second,
		ProbeTimeout:     time.Duration(probeTimeout) * time.Second,
		Scope:            scope,
		Limit:            maxTargets,
		ASNMaxPrefixes:   asnMaxPrefixes,
		EnableIPv6:       enableIPv6,
		DisablePortProbe: disablePortProbe,
		VerifySNI:        enableSNIVerification,
		Verbose:          verbose,
		OutputPath:       out,
	}
	switch {
	case addr != "":
		cfg.TargetKind = scan.TargetKindAddr
		cfg.TargetValue = addr
	case in != "":
		cfg.TargetKind = scan.TargetKindFile
		cfg.TargetValue = in
	default:
		cfg.TargetKind = scan.TargetKindURL
		cfg.TargetValue = url
	}

	engine, err := scan.NewEngine(cfg, stdoutSink{log: logger}, scan.WithLogger(logger))
	if err != nil {
		logger.Error(err.Error())
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if _, err := engine.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("Scan failed", "err", err)
	}
}

type stdoutSink struct {
	log *slog.Logger
}

func (s stdoutSink) OnState(state scan.State, err error) {
	if err != nil {
		s.log.Error("scan state", "state", state, "err", err)
	}
}

func (s stdoutSink) OnProgress(scan.Progress) {}

func (s stdoutSink) OnResult(scan.Result) {}

func (s stdoutSink) OnLog(e scan.LogEntry) {
	attrs := make([]any, 0, len(e.Fields)*2)
	for k, v := range e.Fields {
		attrs = append(attrs, k, v)
	}
	switch e.Level {
	case scan.LogLevelDebug:
		s.log.Debug(e.Message, attrs...)
	case scan.LogLevelWarn:
		s.log.Warn(e.Message, attrs...)
	case scan.LogLevelError:
		s.log.Error(e.Message, attrs...)
	default:
		s.log.Info(e.Message, attrs...)
	}
}
