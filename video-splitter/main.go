package main

import (
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Ép WebView2 tắt tính năng giải mã video bằng GPU (tăng tốc phần cứng)
	// Việc này sửa triệt để lỗi "chỉ có tiếng không có hình" trên mọi video H.264
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--disable-accelerated-video-decode --disable-gpu-video-decode --disable-features=D3D11VideoDecoder")

	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Video Splitter",
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets:  assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewGpuIsDisabled: true,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
