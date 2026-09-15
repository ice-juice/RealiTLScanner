package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/xtls/RealiTLScanner/internal/scan"
)

const geoDBURL = "https://github.com/Loyalsoldier/geoip/releases/latest/download/Country.mmdb"

// ScanRequest is the frontend/Wails binding payload covering all CLI flags.
type ScanRequest struct {
	TargetKind       string `json:"targetKind"`
	TargetValue      string `json:"targetValue"`
	Port             int    `json:"port"`
	ScanThreads      int    `json:"scanThreads"`
	ProbeThreads     int    `json:"probeThreads"`
	TimeoutSec       int    `json:"timeoutSec"`
	ProbeTimeoutSec  int    `json:"probeTimeoutSec"`
	Scope            string `json:"scope"`
	Limit            int    `json:"limit"`
	ASNMaxPrefixes   int    `json:"asnMaxPrefixes"`
	EnableIPv6       bool   `json:"enableIPv6"`
	DisablePortProbe bool   `json:"disablePortProbe"`
	VerifySNI        bool   `json:"verifySni"`
	Verbose          bool   `json:"verbose"`
	OutputPath       string `json:"outputPath"`
	GeoDBPath        string `json:"geoDBPath"`
}

// FieldError is a row-level validation message for the UI.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// GeoStatus describes the local Country.mmdb file.
type GeoStatus struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path"`
	ModTime   string `json:"modTime"`
}

// App is bound to the desktop frontend (Wails or local HTTP).
type App struct {
	ctx     context.Context
	emit    EventEmitter
	dialogs FileDialogs
	store   *settingsStore
	mu      sync.Mutex
	cancel  context.CancelFunc
	busy    bool
}

func NewApp() *App {
	return &App{
		ctx:   context.Background(),
		store: newSettingsStore(),
	}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	raiseFileLimit()
	_ = ensureDir(ConfigDir())
	_ = ensureDir(DefaultOutputDir())
}

func (a *App) SetEmitter(emit EventEmitter) {
	a.emit = emit
}

func (a *App) SetDialogs(dialogs FileDialogs) {
	a.dialogs = dialogs
}

// GetDefaults returns library defaults merged with the last used request.
func (a *App) GetDefaults() ScanRequest {
	def := defaultRequest()
	last := a.store.last()
	if last.TargetValue != "" || last.TargetKind != "" {
		if last.Port == 0 {
			last.Port = def.Port
		}
		if last.ScanThreads == 0 {
			last.ScanThreads = def.ScanThreads
		}
		if last.ProbeThreads == 0 {
			last.ProbeThreads = last.ScanThreads
		}
		if last.TimeoutSec == 0 {
			last.TimeoutSec = def.TimeoutSec
		}
		if last.ProbeTimeoutSec == 0 {
			last.ProbeTimeoutSec = def.ProbeTimeoutSec
		}
		if last.Scope == "" {
			last.Scope = def.Scope
		}
		if last.ASNMaxPrefixes == 0 {
			last.ASNMaxPrefixes = def.ASNMaxPrefixes
		}
		if last.OutputPath == "" {
			last.OutputPath = DefaultOutputPath()
		}
		if last.GeoDBPath == "" {
			last.GeoDBPath = DefaultGeoDBPath()
		}
		return last
	}
	return def
}

func defaultRequest() ScanRequest {
	cfg := scan.DefaultConfig()
	return ScanRequest{
		TargetKind:      string(scan.TargetKindAddr),
		Port:            cfg.Port,
		ScanThreads:     100,
		ProbeThreads:    100,
		TimeoutSec:      int(cfg.Timeout / time.Second),
		ProbeTimeoutSec: int(cfg.ProbeTimeout / time.Second),
		Scope:           string(cfg.Scope),
		Limit:           cfg.Limit,
		ASNMaxPrefixes:  cfg.ASNMaxPrefixes,
		VerifySNI:       true,
		OutputPath:      DefaultOutputPath(),
		GeoDBPath:       DefaultGeoDBPath(),
	}
}

func (a *App) toConfig(req ScanRequest) (scan.Config, error) {
	scope, ok := scan.ParseTargetScope(req.Scope)
	if !ok {
		return scan.Config{}, fmt.Errorf("无效的发现范围，应为 nearby、prefix 或 asn")
	}
	kind := scan.TargetKind(req.TargetKind)
	if kind == "" {
		kind = scan.TargetKindAddr
	}
	out := req.OutputPath
	if out == "" {
		out = DefaultOutputPath()
	}
	geoPath := req.GeoDBPath
	if geoPath == "" {
		geoPath = DefaultGeoDBPath()
	}
	cfg := scan.Config{
		TargetKind:       kind,
		TargetValue:      req.TargetValue,
		Port:             req.Port,
		ScanThreads:      req.ScanThreads,
		ProbeThreads:     req.ProbeThreads,
		Timeout:          time.Duration(req.TimeoutSec) * time.Second,
		ProbeTimeout:     time.Duration(req.ProbeTimeoutSec) * time.Second,
		Scope:            scope,
		Limit:            req.Limit,
		ASNMaxPrefixes:   req.ASNMaxPrefixes,
		EnableIPv6:       req.EnableIPv6,
		DisablePortProbe: req.DisablePortProbe,
		VerifySNI:        req.VerifySNI,
		Verbose:          req.Verbose,
		OutputPath:       out,
		GeoDBPath:        geoPath,
		MaxSeenTargets:   2_000_000,
		MaxSeenDomains:   2_000_000,
	}
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return scan.Config{}, err
	}
	if err := ensureDir(filepath.Dir(cfg.OutputPath)); err != nil {
		return scan.Config{}, fmt.Errorf("无法创建输出目录：%v", err)
	}
	return cfg, nil
}

// ValidateConfig returns field errors for inline UI hints.
func (a *App) ValidateConfig(req ScanRequest) []FieldError {
	_, err := a.toConfig(req)
	if err == nil {
		return nil
	}
	return []FieldError{{Field: "targetValue", Message: err.Error()}}
}

// StartScan starts a single scan task. Returns a Chinese error on conflict or validation failure.
func (a *App) StartScan(req ScanRequest) error {
	cfg, err := a.toConfig(req)
	if err != nil {
		return err
	}
	a.mu.Lock()
	if a.busy {
		a.mu.Unlock()
		return errors.New("已有扫描任务正在运行")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancel = cancel
	a.busy = true
	a.mu.Unlock()

	a.store.remember(req)
	sink := newBatchedSink(a.emit)
	logger := newSinkLogger(sink, req.Verbose)
	eng, err := scan.NewEngine(cfg, sink, scan.WithLogger(logger))
	if err != nil {
		a.finish()
		sink.Close()
		return err
	}

	go func() {
		defer a.finish()
		defer sink.Close()
		summary, runErr := eng.Run(ctx)
		if a.emit != nil {
			a.emit.Emit("scan:done", summary)
		}
		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			logger.Error("scan failed", "err", runErr)
		}
	}()
	return nil
}

func (a *App) finish() {
	a.mu.Lock()
	a.busy = false
	a.cancel = nil
	a.mu.Unlock()
}

// StopScan cancels the running scan. It is idempotent.
func (a *App) StopScan() error {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// PickOutputFile opens a save dialog when Wails dialogs are available.
func (a *App) PickOutputFile() (string, error) {
	if a.dialogs != nil {
		return a.dialogs.SaveCSV(DefaultOutputPath())
	}
	return DefaultOutputPath(), nil
}

// PickInputFile opens an open-file dialog when Wails dialogs are available.
func (a *App) PickInputFile() (string, error) {
	if a.dialogs != nil {
		return a.dialogs.OpenFile(DefaultOutputDir())
	}
	return DefaultOutputDir(), nil
}

// OpenOutputFolder reveals path in Explorer / Finder.
func (a *App) OpenOutputFolder(path string) error {
	if path == "" {
		path = DefaultOutputDir()
	}
	dir := path
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		dir = filepath.Dir(path)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", "/select,", path)
	case "darwin":
		cmd = exec.Command("open", "-R", path)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	return cmd.Start()
}

// GeoDBStatus reports whether Country.mmdb is present.
func (a *App) GeoDBStatus() GeoStatus {
	path := DefaultGeoDBPath()
	st, err := os.Stat(path)
	if err != nil {
		return GeoStatus{Installed: false, Path: path}
	}
	return GeoStatus{
		Installed: true,
		Path:      path,
		ModTime:   st.ModTime().Format(time.RFC3339),
	}
}

// DownloadGeoDB fetches Country.mmdb into the app data directory.
func (a *App) DownloadGeoDB() error {
	if err := ensureDir(ConfigDir()); err != nil {
		return fmt.Errorf("无法创建配置目录：%v", err)
	}
	req, err := http.NewRequestWithContext(a.ctx, http.MethodGet, geoDBURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("下载 GeoIP 数据库失败：%v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载 GeoIP 数据库失败：%s", resp.Status)
	}
	tmp := DefaultGeoDBPath() + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	var downloaded int64
	total := resp.ContentLength
	buf := make([]byte, 32*1024)
	lastEmit := time.Now()
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				_ = f.Close()
				return err
			}
			downloaded += int64(n)
			if a.emit != nil && time.Since(lastEmit) >= 200*time.Millisecond {
				a.emit.Emit("geodb:progress", map[string]int64{"downloaded": downloaded, "total": total})
				lastEmit = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = f.Close()
			return readErr
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if a.emit != nil {
		a.emit.Emit("geodb:progress", map[string]int64{"downloaded": downloaded, "total": total})
	}
	return os.Rename(tmp, DefaultGeoDBPath())
}

func (a *App) ListPresets() []string {
	return a.store.listPresets()
}

func (a *App) SavePreset(name string, req ScanRequest) error {
	if name == "" {
		return errors.New("预设名称不能为空")
	}
	a.store.savePreset(name, req)
	return nil
}

func (a *App) LoadPreset(name string) (ScanRequest, error) {
	req, ok := a.store.loadPreset(name)
	if !ok {
		return ScanRequest{}, fmt.Errorf("找不到预设：%s", name)
	}
	return req, nil
}

func (a *App) DeletePreset(name string) error {
	a.store.deletePreset(name)
	return nil
}

func (a *App) Theme() string {
	return a.store.theme()
}

func (a *App) SetTheme(theme string) {
	a.store.setTheme(theme)
}
