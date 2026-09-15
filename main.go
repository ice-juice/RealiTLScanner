package main

import (
	"context"
	"embed"
	"log"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/xtls/RealiTLScanner/internal/desktop"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := desktop.NewApp()

	err := wails.Run(&options.App{
		Title:            "RealiTLScanner",
		Width:            1280,
		Height:           800,
		MinWidth:         1024,
		MinHeight:        680,
		BackgroundColour: &options.RGBA{R: 15, G: 22, B: 19, A: 255},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: func(ctx context.Context) {
			app.Startup(ctx)
			app.SetEmitter(wailsEmitter{ctx: ctx})
			app.SetDialogs(wailsDialogs{ctx: ctx})
		},
		Bind: []interface{}{app},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			Theme:                windows.Dark,
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			Appearance:           mac.NSAppearanceNameDarkAqua,
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			About: &mac.AboutInfo{
				Title:   "RealiTLScanner",
				Message: "Reality TLS Scanner",
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

type wailsEmitter struct {
	ctx context.Context
}

func (e wailsEmitter) Emit(name string, payload any) {
	runtime.EventsEmit(e.ctx, name, payload)
}

type wailsDialogs struct {
	ctx context.Context
}

func (d wailsDialogs) SaveCSV(defaultPath string) (string, error) {
	return runtime.SaveFileDialog(d.ctx, runtime.SaveDialogOptions{
		DefaultDirectory: filepath.Dir(defaultPath),
		DefaultFilename:  filepath.Base(defaultPath),
		Title:            "保存扫描结果",
		Filters: []runtime.FileFilter{{
			DisplayName: "CSV (*.csv)",
			Pattern:     "*.csv",
		}},
	})
}

func (d wailsDialogs) OpenFile(defaultDir string) (string, error) {
	return runtime.OpenFileDialog(d.ctx, runtime.OpenDialogOptions{
		DefaultDirectory: defaultDir,
		Title:            "选择目标列表",
		Filters: []runtime.FileFilter{{
			DisplayName: "文本 (*.txt;*.csv)",
			Pattern:     "*.txt;*.csv",
		}},
	})
}
