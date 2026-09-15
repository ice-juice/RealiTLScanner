package desktop

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/xtls/RealiTLScanner/internal/scan"
)

const (
	eventFlushInterval = 200 * time.Millisecond
	eventBatchSize     = 50
)

// EventEmitter is implemented by the Wails runtime or the local HTTP/SSE adapter.
type EventEmitter interface {
	Emit(name string, payload any)
}

// FileDialogs is implemented by the Wails runtime; HTTP fallback leaves it nil.
type FileDialogs interface {
	SaveCSV(defaultPath string) (string, error)
	OpenFile(defaultDir string) (string, error)
}

type batchedSink struct {
	emit EventEmitter

	mu      sync.Mutex
	results []scan.Result
	logs    []scan.LogEntry
	closed  bool
}

func newBatchedSink(emit EventEmitter) *batchedSink {
	s := &batchedSink{emit: emit}
	go s.loop()
	return s
}

func (s *batchedSink) loop() {
	ticker := time.NewTicker(eventFlushInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.flush(false)
		s.mu.Lock()
		done := s.closed
		s.mu.Unlock()
		if done {
			s.flush(true)
			return
		}
	}
}

func (s *batchedSink) flush(force bool) {
	s.mu.Lock()
	var results []scan.Result
	var logs []scan.LogEntry
	if force || len(s.results) > 0 {
		results = s.results
		s.results = nil
	}
	if force || len(s.logs) > 0 {
		logs = s.logs
		s.logs = nil
	}
	s.mu.Unlock()
	if len(results) > 0 && s.emit != nil {
		s.emit.Emit("scan:results", results)
	}
	if len(logs) > 0 && s.emit != nil {
		s.emit.Emit("scan:logs", logs)
	}
}

func (s *batchedSink) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}

func (s *batchedSink) OnState(state scan.State, err error) {
	payload := map[string]any{"state": state}
	if err != nil {
		payload["error"] = err.Error()
	}
	if s.emit != nil {
		s.emit.Emit("scan:state", payload)
	}
}

func (s *batchedSink) OnProgress(p scan.Progress) {
	if s.emit != nil {
		s.emit.Emit("scan:progress", p)
	}
}

func (s *batchedSink) OnResult(r scan.Result) {
	s.mu.Lock()
	s.results = append(s.results, r)
	ready := len(s.results) >= eventBatchSize
	s.mu.Unlock()
	if ready {
		s.flush(false)
	}
}

func (s *batchedSink) OnLog(e scan.LogEntry) {
	s.mu.Lock()
	s.logs = append(s.logs, e)
	ready := len(s.logs) >= eventBatchSize
	s.mu.Unlock()
	if ready {
		s.flush(false)
	}
}

type slogSinkHandler struct {
	sink scan.EventSink
	min  slog.Level
}

func (h slogSinkHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.min
}

func (h slogSinkHandler) Handle(_ context.Context, r slog.Record) error {
	fields := map[string]string{}
	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = a.Value.String()
		return true
	})
	level := scan.LogLevelInfo
	switch {
	case r.Level >= slog.LevelError:
		level = scan.LogLevelError
	case r.Level >= slog.LevelWarn:
		level = scan.LogLevelWarn
	case r.Level < slog.LevelInfo:
		level = scan.LogLevelDebug
	}
	h.sink.OnLog(scan.LogEntry{
		Time:    r.Time,
		Level:   level,
		Message: r.Message,
		Fields:  fields,
	})
	return nil
}

func (h slogSinkHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h slogSinkHandler) WithGroup(string) slog.Handler      { return h }

func newSinkLogger(sink scan.EventSink, verbose bool) *slog.Logger {
	min := slog.LevelInfo
	if verbose {
		min = slog.LevelDebug
	}
	return slog.New(slogSinkHandler{sink: sink, min: min})
}
