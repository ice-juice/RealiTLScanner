package scan

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// DefaultTargetLimit is the default -limit value for single-addr discovery.
const DefaultTargetLimit = 4096

// TargetKind selects how the scan target is supplied.
type TargetKind string

const (
	// TargetKindAddr is a single IP, CIDR, or domain.
	TargetKindAddr TargetKind = "addr"
	// TargetKindFile is a line-delimited target list file.
	TargetKindFile TargetKind = "file"
	// TargetKindURL crawls domains from a web page.
	TargetKindURL TargetKind = "url"
)

// TargetScope selects how a single seed address is expanded.
type TargetScope string

const (
	// TargetScopeNearby walks numerically adjacent IPs around the seed.
	TargetScopeNearby TargetScope = "nearby"
	// TargetScopePrefix scans the BGP prefix containing the seed.
	TargetScopePrefix TargetScope = "prefix"
	// TargetScopeASN scans prefixes announced by the seed's ASN.
	TargetScopeASN TargetScope = "asn"
)

// Config is the explicit, immutable-per-run scan configuration.
// Zero values are filled by WithDefaults; Validate returns user-facing Chinese errors.
type Config struct {
	TargetKind  TargetKind
	TargetValue string

	Port         int
	ScanThreads  int
	ProbeThreads int
	Timeout      time.Duration
	ProbeTimeout time.Duration

	Scope          TargetScope
	Limit          int
	ASNMaxPrefixes int
	EnableIPv6     bool

	DisablePortProbe bool
	VerifySNI        bool
	Verbose          bool

	GeoDBPath  string
	OutputPath string

	MaxSeenTargets int
	MaxSeenDomains int
}

// DefaultConfig returns CLI-compatible defaults (thread=2, matching current main.go).
func DefaultConfig() Config {
	return Config{
		Port:           443,
		ScanThreads:    2,
		ProbeThreads:   2,
		Timeout:        10 * time.Second,
		ProbeTimeout:   time.Second,
		Scope:          TargetScopePrefix,
		Limit:          DefaultTargetLimit,
		ASNMaxPrefixes: 32,
		VerifySNI:      true,
		OutputPath:     "out.csv",
	}
}

// WithDefaults fills unset numeric/string fields. Boolean flags and Limit=0 are preserved.
func (c Config) WithDefaults() Config {
	d := DefaultConfig()
	if c.Port <= 0 {
		c.Port = d.Port
	}
	if c.ScanThreads <= 0 {
		c.ScanThreads = d.ScanThreads
	}
	if c.ProbeThreads <= 0 {
		c.ProbeThreads = c.ScanThreads
	}
	if c.Timeout <= 0 {
		c.Timeout = d.Timeout
	}
	if c.ProbeTimeout <= 0 {
		c.ProbeTimeout = d.ProbeTimeout
	}
	if c.Scope == "" {
		c.Scope = d.Scope
	}
	if c.Limit < 0 {
		c.Limit = d.Limit
	}
	if c.ASNMaxPrefixes <= 0 {
		c.ASNMaxPrefixes = d.ASNMaxPrefixes
	}
	if c.OutputPath == "" {
		c.OutputPath = d.OutputPath
	}
	return c
}

// Validate returns a Chinese error suitable for CLI and GUI display.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("配置不能为空")
	}
	kind := c.TargetKind
	if kind == "" {
		return fmt.Errorf("必须指定扫描目标类型（单个目标 / 目标文件 / 网页抓取）")
	}
	switch kind {
	case TargetKindAddr, TargetKindFile, TargetKindURL:
	default:
		return fmt.Errorf("未知的目标类型：%s", kind)
	}
	if strings.TrimSpace(c.TargetValue) == "" {
		return fmt.Errorf("必须指定扫描目标")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("端口必须在 1–65535 之间")
	}
	if c.ScanThreads <= 0 {
		return fmt.Errorf("扫描线程数必须大于 0")
	}
	if c.ProbeThreads <= 0 {
		return fmt.Errorf("探测线程数必须大于 0")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("超时时间必须大于 0")
	}
	if c.ProbeTimeout <= 0 {
		return fmt.Errorf("探测超时必须大于 0")
	}
	if c.Limit < 0 {
		return fmt.Errorf("目标上限不能为负数")
	}
	if c.ASNMaxPrefixes <= 0 {
		return fmt.Errorf("ASN 前缀上限必须大于 0")
	}
	if _, ok := ParseTargetScope(string(c.Scope)); !ok && c.Scope != "" {
		return fmt.Errorf("无效的发现范围，应为 nearby、prefix 或 asn")
	}
	if kind == TargetKindFile {
		st, err := os.Stat(c.TargetValue)
		if err != nil {
			return fmt.Errorf("无法读取目标文件：%s", c.TargetValue)
		}
		if st.IsDir() {
			return fmt.Errorf("目标路径是目录，不是文件：%s", c.TargetValue)
		}
	}
	if kind == TargetKindURL {
		v := strings.TrimSpace(c.TargetValue)
		if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
			return fmt.Errorf("网页地址必须以 http:// 或 https:// 开头")
		}
	}
	if c.MaxSeenTargets < 0 {
		return fmt.Errorf("目标去重上限不能为负数")
	}
	if c.MaxSeenDomains < 0 {
		return fmt.Errorf("域名去重上限不能为负数")
	}
	return nil
}

// ParseTargetScope parses a scope flag value.
func ParseTargetScope(value string) (TargetScope, bool) {
	switch TargetScope(strings.ToLower(strings.TrimSpace(value))) {
	case TargetScopeNearby:
		return TargetScopeNearby, true
	case TargetScopePrefix, "":
		return TargetScopePrefix, true
	case TargetScopeASN:
		return TargetScopeASN, true
	default:
		return "", false
	}
}
