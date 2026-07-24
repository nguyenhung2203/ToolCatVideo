package main

import (
	"embed"
	"video-splitter/internal/browserai"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	browserAIService := browserai.NewService()
	app := NewApp(browserAIService)

	err := wails.Run(&options.App{
		Title:                    "TrafficTool",
		Width:                    1280,
		Height:                   800,
		EnableDefaultContextMenu: false,
		AssetServer: &assetserver.Options{
			Assets:  assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
			browserAIService,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
