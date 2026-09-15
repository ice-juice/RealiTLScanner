package scan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

const progressInterval = 200 * time.Millisecond

// Summary is returned after Engine.Run finishes.
type Summary struct {
	Progress
	OutputPath string
	StoppedBy  State
}

// Engine assembles planner, pipeline, CSV writer, and progress events.
type Engine struct {
	cfg  Config
	sink EventSink
	log  *slog.Logger

	generated   atomic.Int64
	probed      atomic.Int64
	probePassed atomic.Int64
	scanned     atomic.Int64
	found       atomic.Int64
	startedAt   time.Time
	total       int64
}

// EngineOption customizes a new Engine.
type EngineOption func(*Engine)

// WithLogger injects a logger. The library never calls slog.SetDefault.
func WithLogger(log *slog.Logger) EngineOption {
	return func(e *Engine) {
		e.log = loggerOrDiscard(log)
	}
}

// NewEngine validates cfg and prepares an engine. Run performs the scan.
func NewEngine(cfg Config, sink EventSink, opts ...EngineOption) (*Engine, error) {
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if sink == nil {
		sink = NopSink{}
	}
	e := &Engine{
		cfg:  cfg,
		sink: sink,
		log:  loggerOrDiscard(nil),
	}
	for _, opt := range opts {
		opt(e)
	}
	if cfg.Limit > 0 && cfg.TargetKind == TargetKindAddr {
		if _, _, err := net.ParseCIDR(cfg.TargetValue); err != nil {
			e.total = int64(cfg.Limit)
		}
	}
	return e, nil
}

// Run blocks until the scan completes or ctx is cancelled.
// Cancelled runs return context.Canceled after CSV is flushed.
func (e *Engine) Run(ctx context.Context) (summary Summary, err error) {
	e.startedAt = time.Now()
	e.sink.OnState(StatePlanning, nil)

	runDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			select {
			case <-runDone:
			default:
				e.sink.OnState(StateStopping, nil)
			}
		case <-runDone:
		}
	}()
	defer close(runDone)

	outFile, writer, err := e.openOutput()
	if err != nil {
		e.sink.OnState(StateFailed, err)
		return Summary{StoppedBy: StateFailed, OutputPath: e.cfg.OutputPath}, err
	}

	var csvClosed atomic.Bool
	closeCSV := func() {
		if !csvClosed.CompareAndSwap(false, true) {
			return
		}
		if writer != nil {
			if cerr := writer.Close(); cerr != nil && err == nil {
				err = cerr
			}
		}
		if outFile != nil {
			_ = outFile.Close()
		}
	}
	defer closeCSV()
	defer func() {
		if rec := recover(); rec != nil {
			closeCSV()
			pErr := fmt.Errorf("panic: %v", rec)
			e.sink.OnState(StateFailed, pErr)
			panic(rec)
		}
	}()

	geo := NewGeo(e.cfg.GeoDBPath, e.log)
	defer geo.Close()
	domains := NewDomainDeduper(e.cfg.MaxSeenDomains, e.log)

	hosts, cleanup, planErr := e.openHosts(ctx)
	if cleanup != nil {
		defer cleanup()
	}
	if planErr != nil {
		e.sink.OnState(StateFailed, planErr)
		return Summary{StoppedBy: StateFailed, OutputPath: e.cfg.OutputPath}, planErr
	}

	hosts = e.tapGenerated(ctx, hosts)

	progressCtx, stopProgress := context.WithCancel(context.Background())
	defer stopProgress()
	go e.emitProgress(progressCtx)

	e.sink.OnState(StateScanning, nil)
	e.log.Info("Started all scanning threads", "time", e.startedAt)

	tlsOpts := TLSScanOptions{
		Port:       e.cfg.Port,
		Timeout:    e.cfg.Timeout,
		VerifySNI:  e.cfg.VerifySNI,
		EnableIPv6: e.cfg.EnableIPv6,
		Logger:     e.log,
	}
	pipelineOpts := ScanPipelineOptions{
		ProbeThreads: e.cfg.ProbeThreads,
		ScanThreads:  e.cfg.ScanThreads,
		ProbeTimeout: e.cfg.ProbeTimeout,
		EnableIPv6:   e.cfg.EnableIPv6,
		Logger:       e.log,
		Scan: func(scanCtx context.Context, host Host) {
			result, ok := ScanTLSWithOptions(scanCtx, host, geo, domains, tlsOpts)
			e.scanned.Add(1)
			if ok {
				e.found.Add(1)
				writer.Write(result.CSVRow())
				e.sink.OnResult(result)
			}
		},
	}
	if !e.cfg.DisablePortProbe {
		pipelineOpts.Probe = func(probeCtx context.Context, host Host) bool {
			ok := ProbeTCP(probeCtx, host, e.cfg.Port, e.cfg.ProbeTimeout, e.cfg.EnableIPv6, e.log)
			e.probed.Add(1)
			if ok {
				e.probePassed.Add(1)
			}
			return ok
		}
	}

	runErr := RunScanPipeline(ctx, hosts, pipelineOpts)
	stopProgress()
	closeCSV()

	progress := e.snapshot()
	e.sink.OnProgress(progress)
	e.log.Info("Scanning completed", "time", time.Now(), "elapsed", progress.Elapsed.String())

	switch {
	case errors.Is(runErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled):
		e.sink.OnState(StateCancelled, nil)
		return Summary{Progress: progress, OutputPath: e.cfg.OutputPath, StoppedBy: StateCancelled}, context.Canceled
	case runErr != nil:
		e.sink.OnState(StateFailed, runErr)
		return Summary{Progress: progress, OutputPath: e.cfg.OutputPath, StoppedBy: StateFailed}, runErr
	default:
		e.sink.OnState(StateCompleted, nil)
		return Summary{Progress: progress, OutputPath: e.cfg.OutputPath, StoppedBy: StateCompleted}, nil
	}
}

func (e *Engine) openOutput() (*os.File, *CSVResultWriter, error) {
	if e.cfg.OutputPath == "" {
		writer := NewCSVResultWriter(io.Discard)
		return nil, writer, nil
	}
	f, err := os.OpenFile(e.cfg.OutputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("无法打开输出文件：%s", e.cfg.OutputPath)
	}
	writer := NewCSVResultWriter(f)
	writer.Write(CSVHeader)
	return f, writer, nil
}

func (e *Engine) openHosts(ctx context.Context) (<-chan Host, func(), error) {
	switch e.cfg.TargetKind {
	case TargetKindAddr:
		ch := IterateAddrWithOptions(ctx, e.cfg.TargetValue, TargetPlanOptions{
			Scope:          e.cfg.Scope,
			Limit:          e.cfg.Limit,
			EnableIPv6:     e.cfg.EnableIPv6,
			ASNMaxPrefixes: e.cfg.ASNMaxPrefixes,
			MaxSeenTargets: e.cfg.MaxSeenTargets,
			Logger:         e.log,
		})
		return ch, nil, nil
	case TargetKindFile:
		f, err := os.Open(e.cfg.TargetValue)
		if err != nil {
			return nil, nil, fmt.Errorf("无法读取目标文件：%s", e.cfg.TargetValue)
		}
		return Iterate(ctx, f, e.cfg.EnableIPv6, e.log), func() { _ = f.Close() }, nil
	case TargetKindURL:
		domains, err := fetchDomainsFromURL(ctx, e.cfg.TargetValue, e.log)
		if err != nil {
			return nil, nil, err
		}
		return Iterate(ctx, strings.NewReader(strings.Join(domains, "\n")), e.cfg.EnableIPv6, e.log), nil, nil
	default:
		return nil, nil, fmt.Errorf("未知的目标类型：%s", e.cfg.TargetKind)
	}
}

func fetchDomainsFromURL(ctx context.Context, rawURL string, log *slog.Logger) ([]string, error) {
	log = loggerOrDiscard(log)
	log.Info("Fetching url...")
	client := NewNoProxyHTTPClient(30 * time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("无法抓取网页：%v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法抓取网页：%v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("无法读取网页内容：%v", err)
	}
	arr := regexp.MustCompile(`(http|https)://(.*?)[/\"<>\s]+`).FindAllStringSubmatch(string(body), -1)
	var domains []string
	for _, m := range arr {
		domains = append(domains, m[2])
	}
	domains = RemoveDuplicateStr(domains)
	log.Info("Parsed domains", "count", len(domains))
	return domains, nil
}

func (e *Engine) tapGenerated(ctx context.Context, in <-chan Host) <-chan Host {
	out := make(chan Host)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case host, ok := <-in:
				if !ok {
					return
				}
				e.generated.Add(1)
				select {
				case out <- host:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

func (e *Engine) emitProgress(ctx context.Context) {
	ticker := time.NewTicker(progressInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.sink.OnProgress(e.snapshot())
		}
	}
}

func (e *Engine) snapshot() Progress {
	elapsed := time.Since(e.startedAt)
	generated := e.generated.Load()
	rate := 0.0
	if elapsed > 0 {
		rate = float64(generated) / elapsed.Seconds()
	}
	return Progress{
		Generated:   generated,
		Probed:      e.probed.Load(),
		ProbePassed: e.probePassed.Load(),
		Scanned:     e.scanned.Load(),
		Found:       e.found.Load(),
		Elapsed:     elapsed,
		Rate:        rate,
		Total:       e.total,
	}
}
