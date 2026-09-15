package scan

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordSink struct {
	mu        sync.Mutex
	states    []State
	progress  []Progress
	results   []Result
	lastProg  Progress
	hasProg   bool
}

func (s *recordSink) OnState(state State, _ error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states = append(s.states, state)
}

func (s *recordSink) OnProgress(p Progress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.progress = append(s.progress, p)
	s.lastProg = p
	s.hasProg = true
}

func (s *recordSink) OnResult(r Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results = append(s.results, r)
}

func (s *recordSink) OnLog(LogEntry) {}

func (s *recordSink) snapshot() (states []State, p Progress, ok bool, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	states = append([]State(nil), s.states...)
	return states, s.lastProg, s.hasProg, len(s.results)
}

func TestResultCSVRowMatchesHeaderOrder(t *testing.T) {
	r := Result{
		IP:            "10.0.0.1",
		Origin:        "seed.example",
		TLS:           "TLS 1.3",
		ALPN:          "h2",
		Curve:         "X25519",
		CertLength:    "100(certs count: 1)",
		CertSignature: "SHA256-RSA",
		CertPublicKey: "RSA",
		CertDomain:    "cdn.example.com",
		CertIssuer:    "Let's Encrypt",
		GeoCode:       "US",
	}
	row := r.CSVRow()
	if len(row) != len(CSVHeader) {
		t.Fatalf("column count: row=%d header=%d", len(row), len(CSVHeader))
	}
	want := []string{
		r.IP, r.Origin, r.TLS, r.ALPN, r.Curve,
		r.CertLength, r.CertSignature, r.CertPublicKey,
		r.CertDomain, r.CertIssuer, r.GeoCode,
	}
	for i := range want {
		if row[i] != want[i] {
			t.Fatalf("column %d %s: expected %q, got %q", i, CSVHeader[i], want[i], row[i])
		}
	}
}

func TestIterateNearbyHostsCancelClosesChannelQuickly(t *testing.T) {
	baseline := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	ch := IterateNearbyHosts(ctx, netip.MustParseAddr("192.0.2.10"), "192.0.2.10", 0, 0, nil)
	_ = takeHostIPs(t, ch, 8)
	cancel()

	deadline := time.Now().Add(time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				goto closed
			}
		case <-time.After(time.Until(deadline)):
			t.Fatal("generator did not close within 1s after cancel")
		}
	}
closed:
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline+8 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("goroutines did not fall back toward baseline: before=%d after=%d", baseline, runtime.NumGoroutine())
}

func TestEngineRunCancelStopsWithinOneSecond(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.csv")
	cfg := Config{
		TargetKind:       TargetKindAddr,
		TargetValue:      "192.0.2.1",
		Scope:            TargetScopeNearby,
		Limit:            0,
		Port:             1,
		ScanThreads:      2,
		ProbeThreads:     2,
		Timeout:          80 * time.Millisecond,
		ProbeTimeout:     20 * time.Millisecond,
		DisablePortProbe: true,
		OutputPath:       out,
		ASNMaxPrefixes:   1,
	}
	sink := &recordSink{}
	eng, err := NewEngine(cfg, sink)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	baseline := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, runErr := eng.Run(ctx)
		done <- runErr
	}()
	time.Sleep(40 * time.Millisecond)
	cancel()

	select {
	case runErr := <-done:
		if !errors.Is(runErr, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", runErr)
		}
	case <-time.After(time.Second):
		t.Fatal("engine did not stop within 1s")
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline+8 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("goroutines did not fall back toward baseline: before=%d after=%d", baseline, runtime.NumGoroutine())
}

func TestEngineProgressCountsGeneratedAndScanned(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.txt")
	out := filepath.Join(dir, "out.csv")
	if err := os.WriteFile(in, []byte("127.0.0.1\n127.0.0.2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		TargetKind:       TargetKindFile,
		TargetValue:      in,
		Port:             1,
		ScanThreads:      2,
		ProbeThreads:     1,
		Timeout:          80 * time.Millisecond,
		ProbeTimeout:     20 * time.Millisecond,
		DisablePortProbe: true,
		OutputPath:       out,
		ASNMaxPrefixes:   1,
		Scope:            TargetScopePrefix,
	}
	sink := &recordSink{}
	eng, err := NewEngine(cfg, sink)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	summary, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if summary.StoppedBy != StateCompleted {
		t.Fatalf("expected completed, got %s", summary.StoppedBy)
	}
	if summary.Generated != 2 {
		t.Fatalf("expected 2 generated targets, got %d", summary.Generated)
	}
	if summary.Scanned != 2 {
		t.Fatalf("expected 2 scanned targets, got %d", summary.Scanned)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), strings.Join(CSVHeader, ",")) {
		t.Fatalf("expected CSV header in output, got %q", data)
	}
}

func TestEngineWritesHitToCSV(t *testing.T) {
	listener, port := startLocalTLSServer(t, 2)
	defer listener.Close()

	dir := t.TempDir()
	in := filepath.Join(dir, "in.txt")
	out := filepath.Join(dir, "out.csv")
	if err := os.WriteFile(in, []byte("127.0.0.1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		TargetKind:       TargetKindFile,
		TargetValue:      in,
		Port:             port,
		ScanThreads:      1,
		ProbeThreads:     1,
		Timeout:          time.Second,
		ProbeTimeout:     time.Second,
		DisablePortProbe: true,
		VerifySNI:        true,
		OutputPath:       out,
		ASNMaxPrefixes:   1,
		Scope:            TargetScopePrefix,
	}
	sink := &recordSink{}
	eng, err := NewEngine(cfg, sink)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	summary, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if summary.Found != 1 {
		t.Fatalf("expected 1 hit, got %d (scanned=%d)", summary.Found, summary.Scanned)
	}
	_, _, _, n := sink.snapshot()
	if n != 1 {
		t.Fatalf("expected 1 sink result, got %d", n)
	}
}

