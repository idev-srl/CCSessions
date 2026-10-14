package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"golang.org/x/sys/windows/registry"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is set at build time with -ldflags "-X main.version=x.y.z".
var version = "1.0.0"

func main() {
	app := NewApp()

	bg := &options.RGBA{R: 250, G: 250, B: 250, A: 255}
	if systemUsesDarkTheme() {
		bg = &options.RGBA{R: 30, G: 30, B: 32, A: 255}
	}

	err := wails.Run(&options.App{
		Title:            "CC Sessions",
		Width:            1280,
		Height:           820,
		MinWidth:         800,
		MinHeight:        500,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: bg,
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
		Windows: &windows.Options{
			Theme: windows.SystemDefault,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}

// systemUsesDarkTheme reads the Windows "apps" theme so the window
// background matches before the page has loaded.
func systemUsesDarkTheme() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	return err == nil && v == 0
}
