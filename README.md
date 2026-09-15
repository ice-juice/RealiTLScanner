# Reality - TLS - Scanner

扫描公网 TLS 服务，找出适合 Reality 的目标。现提供 **CLI** 与 **桌面原生窗口**（Windows / macOS）。

建议在本地运行；在云端 VPS 上扫描可能导致主机被标记。

## Building

需要 Go 1.26+。

```bash
# CLI
go build -trimpath -ldflags "-s -w" -o RealiTLScanner ./cmd/cli

# 浏览器回退后端（HTTP + SSE）
go build -o RealiTLScanner-desktop ./cmd/desktop
```

Windows（若 `go` 不在 PATH）：

```powershell
& "C:\Program Files\Go\bin\go.exe" build -o dist\RealiTLScanner.exe ./cmd/cli
```

### 桌面原生窗口（Wails v2）

需要 Go 1.26+、Node.js 18+、[Wails v2 CLI](https://wails.io)，以及 Windows 上的 WebView2（Win11 自带）和 C 编译器（如 MinGW / TDM-GCC）。

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails doctor
wails dev
wails build
```

Windows 产物在 `build/bin/RealiTLScanner.exe`，双击即可打开原生窗口。

```powershell
wails build -platform windows/amd64 -ldflags "-s -w"
```

macOS：

```bash
wails build -platform darwin/universal -ldflags "-s -w"
```

无 Wails 时仍可用浏览器回退：

```powershell
& "C:\Program Files\Go\bin\go.exe" run ./cmd/desktop
```

浏览器打开 http://127.0.0.1:34115 。监听地址可用环境变量 `REALITL_DESKTOP_ADDR` 覆盖。

## Usage

必须指定且只能指定 `-addr`、`-in`、`-url` 之一。

```bash
# 显示帮助（未指定目标时打印 flag）
./RealiTLScanner

# 扫描单个 IP / CIDR / 域名
./RealiTLScanner -addr 1.2.3.4

# 从文件读取目标（一行一个）
./RealiTLScanner -in in.txt

# 从网页抓取域名再扫描
./RealiTLScanner -url https://launchpad.net/ubuntu/+archivemirrors

# 端口，默认 443
./RealiTLScanner -addr 1.1.1.1 -port 443

# 详细日志
./RealiTLScanner -addr 1.2.3.0/24 -v

# 输出文件，默认 out.csv
./RealiTLScanner -addr www.microsoft.com -out file.csv

# 并发任务数，默认 2
./RealiTLScanner -addr wiki.ubuntu.com -thread 10

# 单次握手超时（秒），默认 10
./RealiTLScanner -addr 107.172.1.1/16 -timeout 5

# 启用 IPv6
./RealiTLScanner -addr 2001:db8::1 -46

# 单 IP 发现范围
# -scope prefix: 扫描种子 IP 所在 BGP 前缀（默认）
# -scope asn: 扫描该 ASN 宣告的前缀
# -scope nearby: 扫描相邻 IP
./RealiTLScanner -addr 107.172.1.1 -scope prefix -limit 4096 -thread 100

# 连续模式：先扫前缀，再向两侧扩展，直到 Ctrl+C
./RealiTLScanner -addr 107.172.1.1 -scope prefix -limit 0 -thread 100

# ASN 前缀上限
./RealiTLScanner -addr 107.172.1.1 -scope asn -asn-prefixes 32 -limit 10000

# 关闭 TCP 预探测或 SNI 复验
./RealiTLScanner -addr 107.172.1.1 -no-port-probe -verify-sni=false

# 探测超时（秒），默认 1
./RealiTLScanner -addr 107.172.1.1 -probe-timeout 1
```

Ctrl+C / SIGTERM 会取消扫描并 flush CSV。

15 个 CLI flag（名称与默认值保持不变）：

| Flag | 默认 | 含义 |
| --- | --- | --- |
| `-addr` | | 单个 IP / CIDR / 域名 |
| `-in` | | 目标列表文件 |
| `-url` | | 从网页抓取域名 |
| `-port` | `443` | HTTPS 端口 |
| `-thread` | `2` | 探测与扫描共用的并发数 |
| `-out` | `out.csv` | 结果 CSV |
| `-timeout` | `10` | TLS 超时（秒） |
| `-v` | `false` | 详细日志 |
| `-46` | `false` | 启用 IPv6 |
| `-scope` | `prefix` | nearby / prefix / asn |
| `-limit` | `4096` | 单 addr 目标上限；`0` 连续 |
| `-probe-timeout` | `1` | TCP 预探测超时（秒） |
| `-no-port-probe` | `false` | 关闭预探测 |
| `-verify-sni` | `true` | 证书域名 SNI 复验 |
| `-asn-prefixes` | `32` | `-scope asn` 时的前缀上限 |

桌面界面额外提供独立的「探测线程」；CLI 仍用同一个 `-thread` 同时设置探测与扫描线程，行为与重构前一致。

### Docker

```bash
docker build -t realitlscanner .
docker run --rm realitlscanner
docker run --rm realitlscanner -addr 1.1.1.1
```

### Enable Geo IP

将 MaxMind GeoLite2/GeoIP2 Country 数据库放到工作目录，文件名必须为 `Country.mmdb`。可从 [这里](https://github.com/Loyalsoldier/geoip/releases/latest/download/Country.mmdb) 下载。

桌面版会把数据库放在应用配置目录，并可在界面上一键下载。

## CSV

输出固定 11 列（顺序不可改）：

`IP,ORIGIN,TLS,ALPN,CURVE,CERT_LENGTH,CERT_SIGNATURE,CERT_PUBLICKEY,CERT_DOMAIN,CERT_ISSUER,GEO_CODE`

## Demo

Example stdout:

```bash
2024/02/08 20:51:10 INFO Started all scanning threads time=2024-02-08T20:51:10.017+08:00
2024/02/08 20:51:10 INFO Connected to target feasible=true host=107.172.103.9 tls=1.3 alpn=h2 domain=rocky-linux.tk issuer="Let's Encrypt"
2024/02/08 20:51:38 INFO Scanning completed time=2024-02-08T20:51:38.988+08:00 elapsed=28.97043s
```

## 已知限制

- `go test -race` 在未安装 C 编译器的 Windows 上无法运行；CI 的 Linux runner 可以。
- 原生窗口入口是仓库根目录的 `main.go`（Wails v2）。`cmd/desktop` 仍是浏览器 HTTP 回退。
- macOS 代码签名 / 公证需要 Apple Developer 账号。未签名时可用：`xattr -dr com.apple.quarantine /Applications/RealiTLScanner.app`。
- Windows 杀软可能误报未签名的扫描器；可用便携 zip 手工放行。
- Linux 桌面打包本期不做。
- 同一时刻只允许一个扫描任务；结果只持久化为 CSV。
- 长跑去重表默认在桌面端限制约 200 万条，CLI 默认不限制（与原先一致）。
- 后续可迁移 Wails v3，只需改 `cmd/desktop`，`internal/scan` 无需变动。

重构设计见 [`docs/desktop-gui-refactor-plan.md`](docs/desktop-gui-refactor-plan.md)。
