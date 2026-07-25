package main

import (
	"embed"
	"os"
	"path/filepath"
	"video-splitter/internal/browserai"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	browserAIService := browserai.NewService()
	app := NewApp(browserAIService)

	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		configDir = os.TempDir()
	}
	wvDataPath := filepath.Join(configDir, "TrafficTool", "webview2")
	_ = os.MkdirAll(wvDataPath, 0755)

	err = wails.Run(&options.App{
		Title:                    "TrafficTool",
		Width:                    1280,
		Height:                   800,
		EnableDefaultContextMenu: false,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
			browserAIService,
		},
		Windows: &windows.Options{
			WebviewUserDataPath: wvDataPath,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
