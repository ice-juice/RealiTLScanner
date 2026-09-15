# RealiTLScanner 桌面应用重构方案与实现计划

> 目标：把当前的 Go 命令行扫描器重构为 Windows / macOS 上双击即用的桌面应用，
> 用户通过图形界面填写目标与参数，一键启动扫描，实时查看结果，自动输出 CSV。

- 文档版本：v1.0
- 适用代码基线：`main.go` / `planner.go` / `pipeline.go` / `scanner.go` / `utils.go` / `geo.go`
- 预计工期：**11 ~ 15 个工作日**（单人）

---

## 目录

1. [现状分析与阻塞点](#1-现状分析与阻塞点)
2. [目标与非目标](#2-目标与非目标)
3. [技术选型](#3-技术选型)
4. [目标架构](#4-目标架构)
5. [核心库重构详细设计](#5-核心库重构详细设计)
6. [桌面界面设计](#6-桌面界面设计)
7. [分阶段实现计划](#7-分阶段实现计划)
8. [构建、打包与分发](#8-构建打包与分发)
9. [测试策略](#9-测试策略)
10. [风险与对策](#10-风险与对策)
11. [附录 A：核心 API 契约](#附录-a核心-api-契约)
12. [附录 B：前后端事件契约](#附录-b前后端事件契约)

---

## 1. 现状分析与阻塞点

### 1.1 现有架构

当前项目是单一 `package main`，共 6 个源文件，职责划分本身是清晰的：

| 文件 | 职责 | 是否可复用 |
| --- | --- | --- |
| `main.go` | flag 解析、装配、启动 | 否，需重写 |
| `planner.go` | 目标发现（nearby / BGP prefix / ASN），RIPE Stat 客户端 | 是，需改造 |
| `pipeline.go` | 并发管线（TCP 预探测 → TLS 扫描） | 是，需改造 |
| `scanner.go` | TLS 握手、证书解析、SNI 复验、域名去重 | 是，需改造 |
| `utils.go` | 目标解析、CSV 写入、域名校验 | 是，需改造 |
| `geo.go` | GeoIP 国家码查询 | 是，需改造 |

算法层（目标枚举、TLS 判定、去重）质量良好且有测试覆盖，**这部分逻辑一行都不需要重写**，重构的全部工作量在"解耦"和"套壳"。

### 1.2 阻塞点清单

以下 8 项是当前代码直接套 GUI 会失败的原因，必须在 Phase 1 / Phase 2 解决。

#### B1. 核心逻辑直接读取 `main` 包全局变量（阻塞级别：高）

`main.go` 用 `flag.XxxVar` 绑定了 15 个包级全局变量，而核心函数在参数缺省时会**回退读这些全局量**：

| 位置 | 代码 | 读取的全局变量 |
| --- | --- | --- |
| `scanner.go:22` | `opts.Port = port` | `port` |
| `scanner.go:25` | `opts.Timeout = time.Duration(timeout) * time.Second` | `timeout` |
| `utils.go:57` | `if ip != nil && (ip.To4() != nil \|\| enableIPv6)` | `enableIPv6` |
| `utils.go:73` | `if !p.Addr().Is4() && !enableIPv6` | `enableIPv6` |
| `utils.go:135` | `if ip.To4() != nil \|\| enableIPv6` | `enableIPv6` |
| `planner.go:86` | `EnableIPv6: enableIPv6` | `enableIPv6` |

CLI 里进程只跑一次扫描，全局变量无害。GUI 里同一进程会**连续甚至并发**发起多次扫描，全局状态会互相污染：用户第一次勾选 IPv6、第二次取消，行为可能不符合预期。

#### B2. 扫描管线无法取消（阻塞级别：致命）

- `RunScanPipeline(hosts <-chan Host, opts ScanPipelineOptions) error`（`pipeline.go:28`）**不接收 `context`**。
- 目标生成协程 `iterateBoundedHostsWithSeen`（`planner.go:256`）在无缓冲 channel 上执行 `hostChan <- Host{...}`（`planner.go:287`），没有 `select { case <-ctx.Done(): }` 分支。
- `IterateAddrWithOptions` 虽然收了 `ctx`，但只用于 BGP 查询超时（`planner.go:190`），不传递给下游生成器。

后果：GUI 的「停止」按钮**在物理上无法实现**。尤其 `-limit 0` 连续模式下，生成协程永远不退出，只能杀进程。这是必须最先解决的问题。

#### B3. 输出只有 stdout 日志 + 文件两条路（阻塞级别：高）

- `main.go:54-62` 调用 `slog.SetDefault(...)`，这是**进程级全局副作用**。GUI 进程里改默认 logger 会影响 Wails 运行时自身的日志。
- 命中结果通过 `out <- []string{...}`（`scanner.go:86`）写入 CSV channel，非命中信息只以 `slog.Debug` 形式丢弃。
- GUI 需要的是**结构化事件流**（进度 / 命中行 / 日志条目 / 状态变更），而不是文本日志。

#### B4. 没有任何进度信息（阻塞级别：中）

代码中不存在"已扫描数 / 命中数 / 目标总数 / 速率"的计数器。GUI 无法绘制进度条，用户会面对一个"看起来卡死"的界面。

#### B5. GeoIP 数据库路径写死为相对路径（阻塞级别：中）

`geo.go:19`：

```go
reader, err := geoip2.Open("Country.mmdb")
```

相对当前工作目录。macOS 上双击 `.app` 启动时进程 CWD 是 `/`，Windows 上从开始菜单启动时 CWD 也不是应用目录，**GeoIP 必然加载失败**。

#### B6. 长跑内存无上界（阻塞级别：中）

两处 map 只增不减：

- `planner.go:153` 的 `seen map[netip.Addr]struct{}`（目标去重）
- `scanner.go:179` 的 `DomainDeduper.seen map[string]struct{}`（域名去重）

CLI 场景跑几分钟就退出，问题不显；GUI 场景用户可能挂着 `limit=0` 连续扫描数小时，会持续膨胀直至 OOM。

#### B7. 探测与扫描共用同一线程数（阻塞级别：低）

`pipeline.go:38` 和 `pipeline.go:58` 都用 `opts.Threads` 起协程，实际并发是 **2 × Threads**。用户在界面上设 100，实际开 200 个协程、200 条并发连接。需要拆成两个独立可配项，界面上的数字才诚实。

#### B8. 启动时清除代理环境变量（阻塞级别：低）

`main.go:32-35`：

```go
_ = os.Unsetenv("ALL_PROXY")
_ = os.Unsetenv("HTTP_PROXY")
// ...
```

CLI 里这是合理的（避免扫描走代理）。GUI 进程里这会影响 WebView 自身的网络请求。应改为"仅扫描用的 `http.Client` / `net.Dialer` 不走代理"，而非全局清环境变量。

---

## 2. 目标与非目标

### 2.1 目标

| 编号 | 目标 | 验收方式 |
| --- | --- | --- |
| G1 | Windows 10/11 与 macOS 12+ 双击即可运行，无需安装 Go | 在干净虚拟机上验证 |
| G2 | 图形界面完成全部参数配置，无需记忆命令行 flag | 现有 15 个 flag 全部有对应 UI 控件 |
| G3 | 一键开始 / 一键停止，停止在 1 秒内生效 | 手工计时验证 |
| G4 | 实时展示命中结果表格与日志流 | 结果出现后 ≤ 500ms 上屏 |
| G5 | 自动写出 CSV，并可一键打开文件或所在目录 | 功能验证 |
| G6 | 界面美观：跟随系统深/浅色主题，中文界面 | 设计评审 |
| G7 | 保留原 CLI 可用，且行为不回归 | 现有测试全绿 + 手工回归 |

### 2.2 非目标（本期不做）

- Linux 桌面版打包（核心库天然支持，仅 CI 未覆盖，后续可加）
- 移动端
- 扫描任务的持久化队列 / 定时任务
- 多任务并行扫描（本期同一时刻只允许一个扫描任务）
- 结果数据库存储（仍以 CSV 为唯一持久化输出）

---

## 3. 技术选型

### 3.1 候选方案对比

| 方案 | Go 核心复用 | 界面美观度 | 产物体积 | 跨平台成本 | 结论 |
| --- | --- | --- | --- | --- | --- |
| **Wails v2** | 完全复用 | 高（HTML/CSS 全自由） | 10~20 MB | 中（需各平台 CI 构建） | ✅ **选用** |
| Wails v3 | 完全复用 | 高 | 10~20 MB | 中 | ⚠️ 2026-08 起为 Beta，尚未 GA，本期不采用 |
| Fyne | 完全复用 | 中（自绘控件，风格偏统一但朴素） | 25~40 MB | 中（macOS 需 CGo） | 备选 |
| Gio | 完全复用 | 中低（即时模式，控件需自建） | 15~25 MB | 中 | 否 |
| Electron + Go sidecar | 需进程间通信 | 高 | 150+ MB | 低 | 否，体积不可接受 |
| Tauri | **需用 Rust 重写核心** | 高 | 8~15 MB | 中 | 否，工作量不可接受 |

### 3.2 选型结论：Wails v2

**技术栈**

```
后端： Go 1.26+  +  Wails v2
前端： Vue 3 + TypeScript + Vite + Naive UI
```

**选择 Wails v2 的理由**

1. **Go 核心零重写**。扫描算法、BGP 解析、TLS 判定全部原样保留，只做接口层解耦。
2. **界面自由度最高**。用 HTML/CSS 实现"美观"的成本远低于任何自绘 GUI 库。
3. **产物小**。使用系统内置 WebView（Windows 用 WebView2，macOS 用 WKWebView），不打包 Chromium。
4. **原生支持事件推送**。`runtime.EventsEmit` 天然适配实时日志流与结果流，这正是扫描器最核心的交互需求。
5. **稳定**。v2 是当前生产推荐版本；v3 自 2026 年 8 月进入 Beta，桌面 API 虽已稳定但尚未 GA，等 GA 后再评估迁移（届时核心库无需改动，只换 GUI 适配层）。

**选择 Naive UI 的理由**

- Vue 3 + TypeScript 原生支持，类型完整。
- `n-data-table` 内置**虚拟滚动**，这是刚需 —— 扫描可能产出上万行结果，普通表格会卡死。
- 内置深/浅色主题切换，直接满足 G6。
- 按需引入，打包体积可控。

**已知代价（需在实现中处理）**

- Windows 需要 WebView2 Runtime：Win11 内置，Win10 通过 NSIS 安装包内嵌 bootstrapper 自动安装。
- macOS 构建需要 CGo，**不能从 Windows 交叉编译**，必须用 GitHub Actions 的 `macos-latest` runner。
- 开发环境需要 Node.js 18+。

---

## 4. 目标架构

### 4.1 目录结构

```
RealiTLScanner/
├── cmd/
│   ├── cli/
│   │   └── main.go              # 原 CLI 入口（flag 解析 → scan.Config）
│   └── desktop/
│       ├── main.go              # Wails 应用入口
│       ├── app.go               # 绑定给前端的 App 结构体
│       ├── bridge.go            # EventSink 实现：core 事件 → Wails 事件
│       ├── settings.go          # 配置持久化（预设、上次参数）
│       ├── paths.go             # 跨平台路径（配置目录 / 输出目录 / GeoIP）
│       └── build/               # 图标、Info.plist、NSIS 模板
│
├── internal/
│   └── scan/                    # 核心库：无全局变量、可取消、事件化
│       ├── config.go            # Config 定义与校验、默认值
│       ├── types.go             # Host / HostType / Result
│       ├── events.go            # Event / EventSink / Progress / State
│       ├── engine.go            # 【新增】编排器：装配 + 生命周期 + 统计
│       ├── planner.go           # 目标发现（加 ctx）
│       ├── ripestat.go          # 【拆出】RIPE Stat 客户端
│       ├── pipeline.go          # 并发管线（加 ctx，探测/扫描线程分离）
│       ├── tls.go               # 原 scanner.go
│       ├── dedupe.go            # 【拆出】域名去重（加容量上界）
│       ├── geo.go               # GeoIP（路径可配置）
│       ├── csv.go               # CSV 写入器
│       └── *_test.go
│
├── frontend/                    # Wails 前端（Vue 3）
│   ├── src/
│   │   ├── App.vue
│   │   ├── components/
│   │   │   ├── TargetPanel.vue     # 目标输入区
│   │   │   ├── OptionsPanel.vue    # 参数设置区
│   │   │   ├── ResultTable.vue     # 结果表格（虚拟滚动）
│   │   │   ├── LogConsole.vue      # 日志控制台
│   │   │   └── StatusBar.vue       # 底部状态栏
│   │   ├── stores/scan.ts          # Pinia 状态
│   │   └── types/                  # 由 wails 自动生成的绑定类型
│   ├── package.json
│   └── vite.config.ts
│
├── docs/
│   └── desktop-gui-refactor-plan.md   # 本文档
├── wails.json
├── go.mod
└── .github/workflows/release.yml
```

### 4.2 分层原则

```
┌─────────────────────────────────────────┐
│  frontend/  (Vue 3)                     │  只负责渲染与用户输入
└──────────────┬──────────────────────────┘
               │ Wails Binding + Events
┌──────────────┴──────────────────────────┐
│  cmd/desktop/  (GUI 适配层)              │  配置持久化、路径解析、事件桥接
└──────────────┬──────────────────────────┘
               │ scan.Config / scan.EventSink
┌──────────────┴──────────────────────────┐
│  internal/scan/  (核心库)                │  纯 Go，无 UI 依赖，无全局变量
└─────────────────────────────────────────┘
               ▲
               │ scan.Config
┌──────────────┴──────────────────────────┐
│  cmd/cli/  (CLI 适配层)                  │  flag 解析、stdout 日志
└─────────────────────────────────────────┘
```

**铁律：`internal/scan` 不得 import 任何 Wails 包，不得使用 `slog.SetDefault`，不得读取包级可变全局变量。** 这条保证核心库同时服务 CLI 与 GUI，也保证未来迁移 Wails v3 时核心库零改动。

---

## 5. 核心库重构详细设计

### 5.1 配置对象化（解决 B1、B7、B8）

用一个显式 `Config` 替换全部全局变量：

```go
package scan

type TargetKind string

const (
    TargetKindAddr TargetKind = "addr" // 单个 IP / CIDR / 域名
    TargetKindFile TargetKind = "file" // 目标列表文件
    TargetKindURL  TargetKind = "url"  // 从网页抓取域名
)

type Config struct {
    // 目标
    TargetKind  TargetKind
    TargetValue string

    // 扫描参数
    Port         int
    ScanThreads  int           // 对应原 -thread
    ProbeThreads int           // 【新增】拆分自原 -thread，解决 B7
    Timeout      time.Duration
    ProbeTimeout time.Duration

    // 发现策略
    Scope          TargetScope
    Limit          int
    ASNMaxPrefixes int
    EnableIPv6     bool

    // 开关
    DisablePortProbe bool
    VerifySNI        bool
    Verbose          bool

    // 资源与输出
    GeoDBPath  string  // 【新增】解决 B5
    OutputPath string

    // 长跑保护（解决 B6）
    MaxSeenTargets int // 目标去重表上限，0 = 不限
    MaxSeenDomains int // 域名去重表上限，0 = 不限
}

// Validate 返回面向用户的中文错误，供 GUI 直接展示。
func (c *Config) Validate() error

// WithDefaults 填充零值字段，取代原先散落各处的全局变量兜底。
func (c Config) WithDefaults() Config
```

**改造要点**

- 删除 `main.go` 中全部包级变量。
- `NormalizeTLSScanOptions`（`scanner.go:20`）删除对 `port` / `timeout` 的兜底，改由 `Config.WithDefaults()` 统一负责。
- `Iterate` / `LookupIP` 增加 `enableIPv6 bool` 参数，签名改为 `Iterate(r io.Reader, enableIPv6 bool)` / `LookupIP(addr string, enableIPv6 bool)`。
- 删除 `IterateAddr`（`planner.go:83`）这个依赖全局变量的便捷包装，统一走 `IterateAddrWithOptions`。
- 代理隔离：不再 `os.Unsetenv`，改为给扫描用的 dialer 与 RIPE Stat client 显式设置 `Transport.Proxy = nil`。

### 5.2 全链路 context 取消（解决 B2）

这是**优先级最高**的改造。

```go
// pipeline.go
func RunScanPipeline(ctx context.Context, hosts <-chan Host, opts ScanPipelineOptions) error

// planner.go —— 所有生成器都接收 ctx
func IterateNearbyHosts(ctx context.Context, seed netip.Addr, origin string, limit int) <-chan Host
func IteratePrefixHosts(ctx context.Context, prefixes []netip.Prefix, seed netip.Addr, origin string, limit int) <-chan Host
func iterateBoundedHostsWithSeen(ctx context.Context, /* ... */) <-chan Host
func Iterate(ctx context.Context, r io.Reader, enableIPv6 bool) <-chan Host
```

**每一处 channel 发送都必须改成 select**。以 `planner.go:287` 为例：

```go
// 改造前
hostChan <- Host{IP: netIPFromAddr(addr), Origin: originValue, Type: HostTypeIP}

// 改造后
select {
case hostChan <- Host{IP: netIPFromAddr(addr), Origin: originValue, Type: HostTypeIP}:
case <-ctx.Done():
    return false   // 中止生成
}
```

**每一处网络调用都必须挂 ctx**：

- `ProbeTCP`：`net.DialTimeout` → `(&net.Dialer{Timeout: t}).DialContext(ctx, ...)`
- `handshakeTLS`（`scanner.go:116`）：同上，并用 `tls.Client(...).HandshakeContext(ctx)` 替换 `Handshake()`
- `net.LookupIP` → `(&net.Resolver{}).LookupIPAddr(ctx, ...)`

**验收标准**：在 `limit=0` 连续模式下调用 cancel，**1 秒内**所有协程退出（用 `runtime.NumGoroutine()` 在测试中断言）。

### 5.3 事件化输出（解决 B3、B4）

```go
// events.go
package scan

type State string

const (
    StateIdle      State = "idle"
    StatePlanning  State = "planning"  // 正在做 BGP 查询 / 解析目标
    StateScanning  State = "scanning"
    StateStopping  State = "stopping"
    StateCompleted State = "completed"
    StateCancelled State = "cancelled"
    StateFailed    State = "failed"
)

type Progress struct {
    Generated   int64   // 已生成目标数
    Probed      int64   // 已完成 TCP 探测
    ProbePassed int64   // 探测通过
    Scanned     int64   // 已完成 TLS 握手
    Found       int64   // 命中（可用于 Reality）
    Elapsed     time.Duration
    Rate        float64 // 目标/秒
    Total       int64   // 已知总量；连续模式为 0，表示不确定
}

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

func (r Result) CSVRow() []string   // 保证与 csvHeader 顺序严格一致

type LogLevel string

type LogEntry struct {
    Time    time.Time
    Level   LogLevel
    Message string
    Fields  map[string]string
}

// EventSink 由调用方实现：CLI 打到 stdout，GUI 转发到 WebView。
type EventSink interface {
    OnState(State, error)
    OnProgress(Progress)
    OnResult(Result)
    OnLog(LogEntry)
}
```

**关键工程细节：事件节流**

100 线程扫描时命中与日志可能每秒数百条，逐条 `EventsEmit` 会打爆 WebView 的 JS 线程导致界面卡顿。必须在核心库与 GUI 之间做聚合：

- `Progress` 事件固定 **200ms** 发一次（定时器驱动，而非每次计数变化）。
- `Result` 事件在 GUI 适配层缓冲，**每 200ms 或攒够 50 条**批量发送 `scan:results`（数组）。
- `LogEntry` 同样批量发送；前端只保留最近 **2000** 条（环形缓冲），更早的丢弃。
- CSV 写入不走节流，命中即刻落盘，保证进程异常退出也不丢数据。

**日志改造**：`internal/scan` 内部所有 `slog.Info/Debug/Warn` 改为通过注入的 logger 实例输出，不再 `slog.SetDefault`。CLI 侧注入 stdout handler，GUI 侧注入转发到 `EventSink.OnLog` 的 handler。

### 5.4 新增编排器 Engine

把 `main.go:74-157` 的装配逻辑收敛到核心库：

```go
// engine.go
type Engine struct { /* ... */ }

func NewEngine(cfg Config, sink EventSink) (*Engine, error)

// Run 阻塞直到扫描结束或 ctx 取消。
// ctx 取消时返回 context.Canceled，但已命中的结果保证已落盘。
func (e *Engine) Run(ctx context.Context) (Summary, error)

type Summary struct {
    Progress
    OutputPath string
    StoppedBy  State
}
```

`Engine.Run` 负责：装配 planner → pipeline → CSV writer；启动 200ms 进度定时器；维护原子计数器；保证 CSV writer 在**任何**退出路径（正常完成 / 取消 / panic）上都被 `Close()`。

### 5.5 GeoIP 与长跑保护（解决 B5、B6）

```go
// geo.go
func NewGeo(path string, log *slog.Logger) *Geo
```

路径由 GUI 层从应用数据目录解析并传入；为空时降级为 `"N/A"`，不报错。

去重表加上界：达到 `MaxSeenTargets` / `MaxSeenDomains` 后不再新增条目（保守策略：宁可重复扫描，也不 OOM），并发一条 warn 日志提示用户。GUI 默认值设为 200 万条（约占用 100~200MB）。

### 5.6 CLI 适配层

`cmd/cli/main.go` 保留全部现有 flag 与行为，只做三件事：

1. 解析 flag → 构造 `scan.Config`
2. 实现 `stdoutSink`（`OnLog` 打 slog，`OnResult` 不额外输出，`OnProgress` 忽略）
3. 监听 `SIGINT` / `SIGTERM` → `cancel()`，实现优雅停止（**这是顺带得到的能力增强**：现在 Ctrl+C 会正常收尾并 flush CSV）

**兼容性要求**：所有现有 flag 名称、默认值、输出 CSV 列顺序完全不变。

---

## 6. 桌面界面设计

### 6.1 布局

单窗口，默认 1280×800，最小 1024×680。

```
┌───────────────────────────────────────────────────────────────────────┐
│  RealiTLScanner                                    [○ 浅色/深色]  ─ □ ✕ │
├───────────────────────────────────────────────────────────────────────┤
│  扫描目标                                                              │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │ ( ● 单个目标 )  ( ○ 目标文件 )  ( ○ 网页抓取 )                    │  │
│  │ ┌─────────────────────────────────────────┐  [ ▶ 开始扫描 ]      │  │
│  │ │ 38.80.191.155                           │                     │  │
│  │ └─────────────────────────────────────────┘                     │  │
│  │ 支持 IP、CIDR（1.2.3.0/24）或域名                                 │  │
│  └─────────────────────────────────────────────────────────────────┘  │
├──────────────────────┬────────────────────────────────────────────────┤
│  扫描设置             │  [ 命中结果 (128) ]  [ 运行日志 ]               │
│ ┌──────────────────┐ │ ┌────────────────────────────────────────────┐ │
│ │ 端口      443    │ │ │ IP           证书域名        颁发者    地区 │ │
│ │ 扫描线程  ▓▓▓ 100│ │ ├────────────────────────────────────────────┤ │
│ │ 探测线程  ▓▓▓ 100│ │ │ 38.80.191.2  cdn.foo.com  Let's Encrypt US │ │
│ │ 超时(秒)  10     │ │ │ 38.80.191.9  a.bar.net    ZeroSSL       SG │ │
│ │ 目标上限  4096   │ │ │ ...                        （虚拟滚动）     │ │
│ │                  │ │ │                                            │ │
│ │ 发现范围         │ │ │                                            │ │
│ │ (●BGP前缀)(○ASN) │ │ │                                            │ │
│ │ (○邻近IP)        │ │ │                                            │ │
│ │                  │ │ │                                            │ │
│ │ ▾ 高级选项       │ │ │                                            │ │
│ │  ☑ SNI 复验      │ │ │                                            │ │
│ │  ☑ TCP 预探测    │ │ │                                            │ │
│ │  ☐ 启用 IPv6     │ │ │                                            │ │
│ │  ☐ 详细日志      │ │ │                                            │ │
│ │  ASN前缀上限 32  │ │ │                                            │ │
│ │  探测超时     1  │ │ │                                            │ │
│ │                  │ │ │                                            │ │
│ │ 输出文件         │ │ │                                            │ │
│ │ [scan-0907.csv]📁│ │ │                                            │ │
│ │                  │ │ │                                            │ │
│ │ GeoIP: ✓ 已启用  │ │ │                                            │ │
│ └──────────────────┘ │ └────────────────────────────────────────────┘ │
├──────────────────────┴────────────────────────────────────────────────┤
│ ● 扫描中  ▓▓▓▓▓▓▓░░░ 2841/4096  命中 128  1420/s  02:15  [⏹停止][📂打开]│
└───────────────────────────────────────────────────────────────────────┘
```

### 6.2 交互流程

**最简路径（满足 G2「操作简单」）**：打开应用 → 输入一个 IP → 点「开始扫描」。其余全部走默认值（BGP 前缀范围、4096 目标上限、100 线程、自动生成输出文件名）。

**状态机与按钮行为**

| 状态 | 主按钮 | 参数区 | 说明 |
| --- | --- | --- | --- |
| `idle` | 「▶ 开始扫描」 | 可编辑 | 初始状态 |
| `planning` | 「⏹ 停止」 | 只读 | 显示"正在查询 BGP 前缀…" |
| `scanning` | 「⏹ 停止」 | 只读 | 进度条实时更新 |
| `stopping` | 「⏹ 停止」禁用 | 只读 | 显示"正在收尾…" |
| `completed` / `cancelled` | 「▶ 开始扫描」 | 可编辑 | 结果保留，状态栏显示汇总 |
| `failed` | 「▶ 重试」 | 可编辑 | 错误以中文文案展示在状态栏 |

**细节设计**

- 参数区改动**实时校验**并给出行内提示（如线程数 > 500 时警告"可能触发系统连接数限制"）。
- 输出路径默认 `~/Documents/RealiTLScanner/scan-<yyyyMMdd-HHmmss>.csv`，目录不存在时自动创建。
- 结果表格支持按列排序、关键字过滤、右键复制单元格、双击复制整行。
- 扫描完成后状态栏出现「📂 打开文件」和「📁 打开目录」，调用 Wails 的 `BrowserOpenURL` / 系统 `open`、`explorer`。
- 「运行日志」Tab 上有未读红点，出现 warn/error 时高亮。
- 参数预设：可保存 / 载入命名预设，存于配置目录 `presets.json`。应用关闭时自动记住上次参数。

### 6.3 视觉规范（满足 G6「界面美观」）

- **主题**：跟随系统深浅色（Naive UI `darkTheme` + `useOsTheme`），标题栏用 Wails 无边框 + 自绘（macOS 保留红黄绿信号灯位置）。
- **主色**：`#2E7D5B`（深绿，呼应 TLS/安全语义），命中行用主色淡背景标记。
- **字体**：Windows `Microsoft YaHei UI` / macOS `PingFang SC`；IP、域名等技术字段用等宽字体 `JetBrains Mono` / `SF Mono`。
- **间距**：8px 基础栅格。
- **动效**：进度条与数字采用 200ms 缓动过渡，避免高频跳动造成视觉噪音。
- **空状态**：结果表为空时展示插画 + 引导文案，而非空白表格。

### 6.4 GeoIP 引导

界面上显示 GeoIP 状态。未安装时提供「下载 Country.mmdb」按钮，从 `https://github.com/Loyalsoldier/geoip/releases/latest/download/Country.mmdb` 下载到应用数据目录并显示进度。这把原本 README 里的手动步骤变成一次点击。

---

## 7. 分阶段实现计划

每个阶段结束时代码必须可编译、测试全绿，允许随时中断。

### Phase 0 — 准备（0.5 天）

| 任务 | 产出 |
| --- | --- |
| 建立 `feature/desktop-gui` 分支 | 分支 |
| 安装 Wails CLI、Node.js 18+，`wails doctor` 通过 | 环境就绪 |
| 建立 `internal/scan` / `cmd/cli` / `cmd/desktop` 空目录骨架 | 目录结构 |
| 记录当前 `go test ./...` 基线 | 基线报告 |

**验收**：`wails doctor` 全绿；现有测试基线已记录。

---

### Phase 1 — 核心库解耦（2.5 天）⚠️ 关键路径

| 任务 | 涉及问题 |
| --- | --- |
| 将 6 个源文件移入 `internal/scan`，改包名为 `scan`，导出符号加 doc 注释 | — |
| 定义 `Config` / `Validate` / `WithDefaults` | B1 |
| 删除全部包级变量；`Iterate`、`LookupIP` 增加 `enableIPv6` 参数 | B1 |
| `NormalizeTLSScanOptions` 移除全局兜底 | B1 |
| 拆分 `ProbeThreads` / `ScanThreads` | B7 |
| `NewGeo(path)` 支持路径注入 | B5 |
| 从 `planner.go` 拆出 `ripestat.go`，从 `scanner.go` 拆出 `dedupe.go` | 可读性 |
| 扫描 dialer 与 RIPE client 显式禁用代理，删除 `os.Unsetenv` | B8 |
| 迁移全部测试文件到 `package scan`，补充 `Config.Validate` 测试 | — |
| `cmd/cli/main.go` 临时适配，保证 CLI 可跑 | G7 |

**验收**：`go test ./...` 全绿；`go build ./cmd/cli` 成功；`grep` 确认 `internal/scan` 内无包级可变变量；CLI 手工回归三种目标模式（addr / in / url）行为与重构前一致。

---

### Phase 2 — 可取消与事件化（2.5 天）⚠️ 关键路径

| 任务 | 涉及问题 |
| --- | --- |
| 全部生成器函数加 `ctx`，所有 channel 发送改 `select` | B2 |
| `RunScanPipeline` 加 `ctx`；`ProbeTCP`、`handshakeTLS`、DNS 解析改 ctx 版本 | B2 |
| 定义 `events.go`：`State` / `Progress` / `Result` / `LogEntry` / `EventSink` | B3 B4 |
| 新增 `engine.go`：装配、原子计数器、200ms 进度定时器、退出路径保证 CSV Close | B3 B4 |
| 内部 `slog` 调用改为注入 logger | B3 |
| 去重表加容量上界 | B6 |
| CLI 接入 `SIGINT` 优雅停止 | 增强 |
| 新增测试：取消后 1s 内协程归零、进度计数正确、Result→CSVRow 列序一致 | — |

**验收**：`limit=0` 连续模式下 cancel，1 秒内 `runtime.NumGoroutine()` 回落至基线；`go test -race ./...` 全绿；CLI Ctrl+C 能正常收尾并 flush CSV。

> **里程碑**：Phase 2 结束时，核心库已完全具备 GUI 所需的全部能力。后续阶段即使延期，CLI 也已获得可取消、有进度、无全局状态的改进。

---

### Phase 3 — Wails 骨架与桥接（2 天）

| 任务 |
| --- |
| `wails init` 生成 Vue 3 + TS 模板，合入现有仓库 |
| `cmd/desktop/app.go`：绑定 `StartScan(cfg) error` / `StopScan()` / `GetDefaults()` / `PickOutputFile()` / `OpenOutputFolder()` / `DownloadGeoDB()` |
| `cmd/desktop/bridge.go`：实现 `EventSink`，含结果与日志的 200ms 批量聚合 |
| `cmd/desktop/paths.go`：跨平台配置目录、默认输出目录、GeoIP 路径 |
| `cmd/desktop/settings.go`：参数持久化与预设 |
| 前端最小页面：一个输入框 + 开始/停止 + 纯文本日志区，跑通端到端 |

**验收**：`wails dev` 下输入 IP 能启动真实扫描，日志实时上屏，点停止 1 秒内停下，CSV 正确写出。

---

### Phase 4 — 界面实现（3.5 天）

| 任务 |
| --- |
| `TargetPanel.vue`：三种目标模式切换 + 行内校验 |
| `OptionsPanel.vue`：基础参数 + 高级折叠区，全部 15 个 flag 覆盖 |
| `ResultTable.vue`：`n-data-table` 虚拟滚动、排序、过滤、复制 |
| `LogConsole.vue`：级别着色、自动滚动开关、2000 条环形缓冲、关键字过滤 |
| `StatusBar.vue`：状态灯、进度条、命中数、速率、耗时、操作按钮 |
| Pinia store 统一管理扫描状态与事件订阅 |
| 主题跟随系统 + 手动切换；无边框窗口与拖拽区 |
| 空状态、错误态、GeoIP 引导下载 |
| 全量中文文案与参数说明 tooltip |

**验收**：设计评审通过；15 个 flag 全部有 UI 对应；1 万行结果表格滚动不掉帧。

---

### Phase 5 — 打包与分发（2 天）

| 任务 |
| --- |
| 应用图标（Windows `.ico` / macOS `.icns`）、`Info.plist`、版本号注入 |
| Windows：`wails build -nsis`，WebView2 bootstrapper 内嵌 |
| macOS：`darwin/universal` 通用二进制；`codesign` + `notarytool` 公证脚本 |
| 改造 `.github/workflows/release.yml` 为 matrix：`windows-latest` + `macos-latest` + 原有 CLI 跨平台产物 |
| 提升文件描述符上限（macOS 默认 256，`setrlimit` 提到 8192） |
| 首次运行引导与 README 更新 |

**验收**：干净 Win10 / Win11 / macOS（Intel + Apple Silicon）虚拟机上双击运行成功；macOS 无 Gatekeeper 拦截；打 tag 后 CI 自动产出全部资产。

---

### Phase 6 — 收尾（1 天）

| 任务 |
| --- |
| 端到端手工测试清单执行 |
| 性能压测：1 万目标 / 500 线程，观察内存与界面响应 |
| README 补充桌面版章节与截图 |
| 已知问题与后续路线（Linux 打包、Wails v3 迁移）记录 |

---

### 工期汇总

| 阶段 | 工作量 | 累计 |
| --- | --- | --- |
| Phase 0 准备 | 0.5 天 | 0.5 |
| Phase 1 核心库解耦 | 2.5 天 | 3.0 |
| Phase 2 可取消与事件化 | 2.5 天 | 5.5 |
| Phase 3 Wails 骨架 | 2.0 天 | 7.5 |
| Phase 4 界面实现 | 3.5 天 | 11.0 |
| Phase 5 打包分发 | 2.0 天 | 13.0 |
| Phase 6 收尾 | 1.0 天 | 14.0 |

**合计约 14 个工作日**，考虑 macOS 签名公证等不确定因素，预留至 15 天。

---

## 8. 构建、打包与分发

### 8.1 开发环境

```powershell
# Windows
winget install GoLang.Go
winget install OpenJS.NodeJS.LTS
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails doctor
```

```bash
# macOS
brew install go node
xcode-select --install
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails doctor
```

### 8.2 构建命令

```bash
# 开发热重载
wails dev

# Windows 安装包（内嵌 WebView2 bootstrapper）
wails build -platform windows/amd64 -nsis -ldflags "-s -w"

# macOS 通用二进制
wails build -platform darwin/universal -ldflags "-s -w"

# CLI 仍按原方式构建
go build -trimpath -ldflags "-s -w" -o RealiTLScanner ./cmd/cli
```

### 8.3 CI 改造

`release.yml` 由单 job 改为三 job：

| Job | Runner | 产出 |
| --- | --- | --- |
| `cli` | `ubuntu-latest` | 现有 8 个平台的 CLI 压缩包（逻辑不变，仅入口改为 `./cmd/cli`） |
| `desktop-windows` | `windows-latest` | `RealiTLScanner-Setup-<ver>.exe`、便携版 `.zip` |
| `desktop-macos` | `macos-latest` | 签名公证后的 `RealiTLScanner-<ver>.dmg` |

三者产物汇总后统一生成 `SHA256SUMS.txt` 并发 Release。

### 8.4 分发注意事项

- **Windows**：扫描器类程序易被杀软误报。建议申请代码签名证书；未签名时在 README 说明如何添加信任。
- **macOS**：必须 `codesign` + `notarytool` 公证，否则用户会遇到"应用已损坏"。需要 Apple Developer 账号（$99/年）。临时规避方案（README 中说明）：`xattr -dr com.apple.quarantine /Applications/RealiTLScanner.app`。
- **免责声明**：首次启动弹出一次性提示，说明工具用途与"建议本地运行，云端 VPS 运行可能被标记"（沿用现有 README 警告）。

---

## 9. 测试策略

| 层级 | 范围 | 工具 |
| --- | --- | --- |
| 单元测试 | `internal/scan` 全部逻辑；现有 5 个测试文件迁移后必须全绿 | `go test` |
| 竞态检测 | 并发管线、计数器、去重表 | `go test -race` |
| 取消测试 | cancel 后 1s 内协程归零；CSV 已 flush | `runtime.NumGoroutine()` 断言 |
| 契约测试 | `Result.CSVRow()` 与 `csvHeader` 列序一致（防止字段增删错位） | 表驱动测试 |
| 集成测试 | 本地起 TLS server，验证端到端命中判定 | `httptest` + 自签证书 |
| 手工测试 | 三种目标模式 × 三种发现范围 × 启停组合 | 测试清单 |
| 兼容性测试 | Win10 / Win11 / macOS Intel / macOS Apple Silicon | 虚拟机 |
| 性能测试 | 1 万目标 / 500 线程下的内存占用与 UI 帧率 | 手工观测 |

**回归红线**：CSV 输出的 11 列名称与顺序、全部 CLI flag 名称与默认值，在整个重构中不得改变。

---

## 10. 风险与对策

| # | 风险 | 影响 | 概率 | 对策 |
| --- | --- | --- | --- | --- |
| R1 | 全链路加 ctx 时遗漏某处 channel 发送，导致停止后仍有协程泄漏 | 高 | 中 | 用协程数断言测试兜底；code review 逐一核对每个 `<-` 发送点 |
| R2 | 高频事件打爆 WebView，界面卡顿 | 高 | 中高 | 200ms 批量聚合 + 前端环形缓冲 + 虚拟滚动，三重防护；Phase 3 即压测 |
| R3 | macOS 签名公证流程受阻（无开发者账号） | 中 | 中 | 提前申请；未就绪时先发未签名版并在 README 给 `xattr` 方案 |
| R4 | Windows 杀软误报导致用户无法运行 | 中 | 中 | 代码签名；README 说明；提供便携 zip 供手工放行 |
| R5 | 高线程数触发系统连接数 / 文件描述符限制 | 中 | 高 | macOS 启动时 `setrlimit` 提至 8192；界面对 >500 线程给出警告；文档说明 |
| R6 | 重构引入行为回归，CLI 用户受影响 | 高 | 低 | 现有测试全部迁移保留；flag 与 CSV 列作为红线；Phase 1 结束即做 CLI 回归 |
| R7 | 连续模式长跑内存增长 | 中 | 中 | 去重表容量上界；界面显示内存占用 |
| R8 | Wails v3 GA 后 v2 逐渐停止维护 | 低 | 中 | 核心库与 GUI 严格分层，迁移只需重写 `cmd/desktop`，工作量约 2 天 |

---

## 附录 A：核心 API 契约

`internal/scan` 对外暴露的完整接口（GUI 与 CLI 都只依赖这些）：

```go
package scan

// ---- 配置 ----
type TargetKind string
type TargetScope string

type Config struct { /* 见 5.1 */ }

func (c Config) WithDefaults() Config
func (c *Config) Validate() error
func DefaultConfig() Config

// ---- 事件 ----
type State string
type Progress struct { /* 见 5.3 */ }
type Result struct { /* 见 5.3 */ }
type LogEntry struct { /* 见 5.3 */ }

type EventSink interface {
    OnState(state State, err error)
    OnProgress(p Progress)
    OnResult(r Result)
    OnLog(e LogEntry)
}

// ---- 执行 ----
type Engine struct{ /* unexported */ }
type Summary struct { /* 见 5.4 */ }

func NewEngine(cfg Config, sink EventSink) (*Engine, error)
func (e *Engine) Run(ctx context.Context) (Summary, error)

// ---- 辅助 ----
var CSVHeader = []string{
    "IP", "ORIGIN", "TLS", "ALPN", "CURVE",
    "CERT_LENGTH", "CERT_SIGNATURE", "CERT_PUBLICKEY",
    "CERT_DOMAIN", "CERT_ISSUER", "GEO_CODE",
}

func (r Result) CSVRow() []string
func ParseTargetScope(v string) (TargetScope, bool)
```

---

## 附录 B：前后端事件契约

### B.1 前端调用后端（Wails Binding）

| 方法 | 签名 | 说明 |
| --- | --- | --- |
| `StartScan` | `(cfg ScanRequest) error` | 校验失败时返回中文错误；已有任务运行时返回冲突错误 |
| `StopScan` | `() error` | 幂等；触发 cancel |
| `GetDefaults` | `() ScanRequest` | 返回默认值 + 上次使用的参数 |
| `ValidateConfig` | `(cfg ScanRequest) []FieldError` | 供前端行内校验 |
| `PickOutputFile` | `() (string, error)` | 系统保存文件对话框 |
| `PickInputFile` | `() (string, error)` | 系统打开文件对话框 |
| `OpenOutputFolder` | `(path string) error` | 在资源管理器 / Finder 中定位文件 |
| `GeoDBStatus` | `() GeoStatus` | 返回是否已安装、路径、文件日期 |
| `DownloadGeoDB` | `() error` | 下载 Country.mmdb，进度经事件推送 |
| `ListPresets` / `SavePreset` / `DeletePreset` | — | 参数预设管理 |

### B.2 后端推送前端（Wails Events）

| 事件名 | 载荷 | 频率 |
| --- | --- | --- |
| `scan:state` | `{ state, error? }` | 状态变更时 |
| `scan:progress` | `Progress` | 每 200ms |
| `scan:results` | `Result[]` | 每 200ms 或攒够 50 条 |
| `scan:logs` | `LogEntry[]` | 每 200ms 或攒够 50 条 |
| `scan:done` | `Summary` | 结束一次 |
| `geodb:progress` | `{ downloaded, total }` | 下载中每 200ms |

---

## 变更记录

| 版本 | 日期 | 说明 |
| --- | --- | --- |
| v1.0 | 2026-09-07 | 初版：现状分析、Wails v2 选型、六阶段实现计划 |
