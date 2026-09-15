package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const appName = "RealiTLScanner"

// ConfigDir is the platform-specific directory for settings and GeoIP.
func ConfigDir() string {
	if dir := os.Getenv("REALITL_CONFIG_DIR"); dir != "" {
		return dir
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, appName)
	}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", appName)
	}
	return filepath.Join(home, ".config", appName)
}

// DefaultOutputDir is ~/Documents/RealiTLScanner.
func DefaultOutputDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Documents", appName)
}

// DefaultOutputPath returns a timestamped CSV path under DefaultOutputDir.
func DefaultOutputPath() string {
	name := "scan-" + time.Now().Format("20060102-150405") + ".csv"
	return filepath.Join(DefaultOutputDir(), name)
}

// DefaultGeoDBPath is Country.mmdb inside the config directory.
func DefaultGeoDBPath() string {
	return filepath.Join(ConfigDir(), "Country.mmdb")
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}
