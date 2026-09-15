package scan

import (
	"context"
	"net"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestRunScanPipelineProbesBeforeTLSScan(t *testing.T) {
	hosts := make(chan Host, 3)
	hosts <- Host{IP: net.ParseIP("192.0.2.10"), Origin: "closed", Type: HostTypeIP}
	hosts <- Host{IP: net.ParseIP("192.0.2.11"), Origin: "open", Type: HostTypeIP}
	hosts <- Host{IP: net.ParseIP("192.0.2.12"), Origin: "closed2", Type: HostTypeIP}
	close(hosts)

	var mu sync.Mutex
	var scanned []string
	err := RunScanPipeline(context.Background(), hosts, ScanPipelineOptions{
		Threads: 2,
		Probe: func(_ context.Context, host Host) bool {
			return host.Origin == "open"
		},
		Scan: func(_ context.Context, host Host) {
			mu.Lock()
			defer mu.Unlock()
			scanned = append(scanned, host.Origin)
		},
	})
	if err != nil {
		t.Fatalf("run scan pipeline: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(scanned, []string{"open"}) {
		t.Fatalf("expected only open host to be scanned, got %#v", scanned)
	}
}

func TestNormalizePipelineThreadsDefaultsToOne(t *testing.T) {
	opts := NormalizeScanPipelineOptions(ScanPipelineOptions{Threads: 0})

	if opts.Threads != 1 {
		t.Fatalf("expected thread count to default to 1, got %d", opts.Threads)
	}
	if opts.ProbeThreads != 1 || opts.ScanThreads != 1 {
		t.Fatalf("expected probe/scan threads to default to 1, got probe=%d scan=%d", opts.ProbeThreads, opts.ScanThreads)
	}
	if opts.ProbeTimeout != time.Second {
		t.Fatalf("expected default probe timeout of 1s, got %s", opts.ProbeTimeout)
	}
}

func TestNormalizePipelineSplitsProbeAndScanThreads(t *testing.T) {
	opts := NormalizeScanPipelineOptions(ScanPipelineOptions{ProbeThreads: 3, ScanThreads: 7})
	if opts.ProbeThreads != 3 || opts.ScanThreads != 7 {
		t.Fatalf("expected split threads to be preserved, got probe=%d scan=%d", opts.ProbeThreads, opts.ScanThreads)
	}
}

func TestProbeTCPDetectsOpenAndClosedPorts(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split listener address: %v", err)
	}
	openPort, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse listener port: %v", err)
	}
	host := Host{IP: net.ParseIP("127.0.0.1"), Origin: "127.0.0.1", Type: HostTypeIP}
	if !ProbeTCP(context.Background(), host, openPort, time.Second, false, nil) {
		t.Fatal("expected open port probe to succeed")
	}

	closedListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for closed port: %v", err)
	}
	_, closedPortText, err := net.SplitHostPort(closedListener.Addr().String())
	if err != nil {
		t.Fatalf("split closed listener address: %v", err)
	}
	closedPort, err := strconv.Atoi(closedPortText)
	if err != nil {
		t.Fatalf("parse closed listener port: %v", err)
	}
	_ = closedListener.Close()
	if ProbeTCP(context.Background(), host, closedPort, 50*time.Millisecond, false, nil) {
		t.Fatal("expected closed port probe to fail")
	}
}
