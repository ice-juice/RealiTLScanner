package scan

import "time"

// State is the scan lifecycle state.
type State string

const (
	StateIdle      State = "idle"
	StatePlanning  State = "planning"
	StateScanning  State = "scanning"
	StateStopping  State = "stopping"
	StateCompleted State = "completed"
	StateCancelled State = "cancelled"
	StateFailed    State = "failed"
)

// Progress is a periodic snapshot of scan counters.
type Progress struct {
	Generated   int64
	Probed      int64
	ProbePassed int64
	Scanned     int64
	Found       int64
	Elapsed     time.Duration
	Rate        float64
	Total       int64
}

// Result is one feasible Reality-compatible hit.
type Result struct {
	IP            string `json:"ip"`
	Origin        string `json:"origin"`
	TLS           string `json:"tls"`
	ALPN          string `json:"alpn"`
	Curve         string `json:"curve"`
	CertLength    string `json:"certLength"`
	CertSignature string `json:"certSignature"`
	CertPublicKey string `json:"certPublicKey"`
	CertDomain    string `json:"certDomain"`
	CertIssuer    string `json:"certIssuer"`
	GeoCode       string `json:"geoCode"`
}

// CSVRow returns columns in the same order as CSVHeader.
func (r Result) CSVRow() []string {
	return []string{
		r.IP,
		r.Origin,
		r.TLS,
		r.ALPN,
		r.Curve,
		r.CertLength,
		r.CertSignature,
		r.CertPublicKey,
		r.CertDomain,
		r.CertIssuer,
		r.GeoCode,
	}
}

// LogLevel is a structured log severity.
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// LogEntry is a structured log line for CLI or GUI.
type LogEntry struct {
	Time    time.Time
	Level   LogLevel
	Message string
	Fields  map[string]string
}

// EventSink is implemented by CLI (stdout) and the desktop adapter.
type EventSink interface {
	OnState(State, error)
	OnProgress(Progress)
	OnResult(Result)
	OnLog(LogEntry)
}

// NopSink discards all events.
type NopSink struct{}

func (NopSink) OnState(State, error) {}
func (NopSink) OnProgress(Progress)  {}
func (NopSink) OnResult(Result)      {}
func (NopSink) OnLog(LogEntry)       {}
