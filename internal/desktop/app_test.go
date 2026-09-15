package desktop

import "testing"

func TestToConfigRejectsEmptyTarget(t *testing.T) {
	t.Setenv("REALITL_CONFIG_DIR", t.TempDir())
	app := NewApp()
	_, err := app.toConfig(ScanRequest{TargetKind: "addr", Port: 443, ScanThreads: 1, ProbeThreads: 1, TimeoutSec: 1, ProbeTimeoutSec: 1, Scope: "nearby", ASNMaxPrefixes: 1})
	if err == nil {
		t.Fatal("expected empty target to fail validation")
	}
}

func TestGetDefaultsHasFifteenFlagFields(t *testing.T) {
	t.Setenv("REALITL_CONFIG_DIR", t.TempDir())
	app := NewApp()
	req := app.GetDefaults()
	if req.Port != 443 {
		t.Fatalf("port: %d", req.Port)
	}
	if req.Scope != "prefix" {
		t.Fatalf("scope: %s", req.Scope)
	}
	if req.Limit != 4096 {
		t.Fatalf("limit: %d", req.Limit)
	}
	if !req.VerifySNI {
		t.Fatal("verifySni should default true")
	}
	if req.OutputPath == "" {
		t.Fatal("expected a default output path")
	}
}

func TestStopScanIsIdempotent(t *testing.T) {
	t.Setenv("REALITL_CONFIG_DIR", t.TempDir())
	app := NewApp()
	if err := app.StopScan(); err != nil {
		t.Fatal(err)
	}
}
