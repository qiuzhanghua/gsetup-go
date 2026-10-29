package main

import (
	"embed"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	wailsApp := application.New(application.Options{
		Name:        "GUI-Setup",
		Description: "给 gpm 打包出来的 GUI 应用配的图形安装向导",
		Services: []application.Service{
			application.NewService(app),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	// 主窗口：尺寸与 v2 时一致，交给 v3 的窗口管理器在 Run 时创建。
	wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "GUI-Setup",
		Width:     860,
		Height:    680,
		MinWidth:  720,
		MinHeight: 520,
	})

	if err := wailsApp.Run(); err != nil {
		println("Error:", err.Error())
	}
}
