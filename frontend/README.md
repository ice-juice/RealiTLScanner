# 桌面前端

Vue 3 + TypeScript + Pinia + Naive UI。原生窗口由仓库根目录的 Wails v2 入口 `main.go` 加载 `frontend/dist`。

## 开发

在仓库根目录：

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails doctor
wails dev
```

只构建前端：

```bash
npm install
npm run build
```

Wails 绑定在 `window.go.desktop.App`。`src/stores/scan.ts` 同时支持 Wails Events 与 `/api/events` SSE。

## 浏览器回退

未执行 `wails build` 时：

```powershell
& "C:\Program Files\Go\bin\go.exe" run ./cmd/desktop
```

打开 http://127.0.0.1:34115 ，使用 `fallback.html`（或已构建的 `dist`）。
