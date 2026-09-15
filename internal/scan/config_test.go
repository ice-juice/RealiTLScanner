package scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfigMatchesCLIDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Port != 443 {
		t.Fatalf("port: got %d", cfg.Port)
	}
	if cfg.ScanThreads != 2 || cfg.ProbeThreads != 2 {
		t.Fatalf("threads: scan=%d probe=%d", cfg.ScanThreads, cfg.ProbeThreads)
	}
	if cfg.Timeout != 10*time.Second {
		t.Fatalf("timeout: got %s", cfg.Timeout)
	}
	if cfg.ProbeTimeout != time.Second {
		t.Fatalf("probe timeout: got %s", cfg.ProbeTimeout)
	}
	if cfg.Scope != TargetScopePrefix {
		t.Fatalf("scope: got %s", cfg.Scope)
	}
	if cfg.Limit != DefaultTargetLimit {
		t.Fatalf("limit: got %d", cfg.Limit)
	}
	if cfg.ASNMaxPrefixes != 32 {
		t.Fatalf("asn prefixes: got %d", cfg.ASNMaxPrefixes)
	}
	if !cfg.VerifySNI {
		t.Fatal("expected verify SNI to default true")
	}
	if cfg.OutputPath != "out.csv" {
		t.Fatalf("output: got %s", cfg.OutputPath)
	}
	if cfg.EnableIPv6 || cfg.DisablePortProbe || cfg.Verbose {
		t.Fatal("expected boolean flags to default false")
	}
}

func TestWithDefaultsFillsZerosAndKeepsUnlimitedLimit(t *testing.T) {
	cfg := Config{TargetKind: TargetKindAddr, TargetValue: "1.2.3.4", Limit: 0}.WithDefaults()
	if cfg.Port != 443 || cfg.ScanThreads != 2 {
		t.Fatalf("expected numeric defaults, got port=%d threads=%d", cfg.Port, cfg.ScanThreads)
	}
	if cfg.Limit != 0 {
		t.Fatalf("expected unlimited limit to be preserved, got %d", cfg.Limit)
	}
	if cfg.ProbeThreads != cfg.ScanThreads {
		t.Fatalf("expected probe threads to follow scan threads, got %d vs %d", cfg.ProbeThreads, cfg.ScanThreads)
	}
}

func TestConfigValidate(t *testing.T) {
	validFile := filepath.Join(t.TempDir(), "targets.txt")
	if err := os.WriteFile(validFile, []byte("1.2.3.4\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name:    "missing kind",
			cfg:     Config{TargetValue: "1.2.3.4"},
			wantErr: "目标类型",
		},
		{
			name:    "missing value",
			cfg:     Config{TargetKind: TargetKindAddr},
			wantErr: "必须指定扫描目标",
		},
		{
			name: "bad port",
			cfg: Config{
				TargetKind: TargetKindAddr, TargetValue: "1.2.3.4",
				Port: 70000, ScanThreads: 1, ProbeThreads: 1,
				Timeout: time.Second, ProbeTimeout: time.Second,
				ASNMaxPrefixes: 1, Scope: TargetScopePrefix,
			},
			wantErr: "端口",
		},
		{
			name: "bad file",
			cfg: Config{
				TargetKind: TargetKindFile, TargetValue: filepath.Join(t.TempDir(), "missing.txt"),
				Port: 443, ScanThreads: 1, ProbeThreads: 1,
				Timeout: time.Second, ProbeTimeout: time.Second,
				ASNMaxPrefixes: 1, Scope: TargetScopePrefix,
			},
			wantErr: "目标文件",
		},
		{
			name: "bad url",
			cfg: Config{
				TargetKind: TargetKindURL, TargetValue: "ftp://example.com",
				Port: 443, ScanThreads: 1, ProbeThreads: 1,
				Timeout: time.Second, ProbeTimeout: time.Second,
				ASNMaxPrefixes: 1, Scope: TargetScopePrefix,
			},
			wantErr: "http",
		},
		{
			name: "ok addr",
			cfg: Config{
				TargetKind: TargetKindAddr, TargetValue: "1.2.3.4",
				Port: 443, ScanThreads: 1, ProbeThreads: 1,
				Timeout: time.Second, ProbeTimeout: time.Second,
				ASNMaxPrefixes: 1, Scope: TargetScopeNearby,
			},
		},
		{
			name: "ok file",
			cfg: Config{
				TargetKind: TargetKindFile, TargetValue: validFile,
				Port: 443, ScanThreads: 1, ProbeThreads: 1,
				Timeout: time.Second, ProbeTimeout: time.Second,
				ASNMaxPrefixes: 1, Scope: TargetScopePrefix,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
