package main

import (
	"embed"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed frontend/public/favicon.ico
var trayIcon []byte

func frontendBootstrapHTML() string {
	target := strings.TrimRight(os.Getenv("FRONTEND_DEVSERVER_URL"), "/")
	if target == "" {
		target = "http://wails.localhost"
	}
	return "<!doctype html><html><head><meta charset=\"UTF-8\"><script>location.replace(" + strconv.Quote(target+"/") + ")</script></head><body></body></html>"
}

func toggleWindow(window application.Window) {
	if window.IsVisible() {
		window.Hide()
		return
	}
	window.Show().Focus()
}

func main() {
	if err := installInputLockHook(); err != nil {
		log.Fatal(err)
	}
	service := NewYtLiteService()
	app := application.New(application.Options{
		Name:        "YTLite",
		Description: "A lightweight YouTube desktop player",
		Icon:        trayIcon,
		Services:    []application.Service{application.NewService(service)},
		Assets:      application.AssetOptions{Handler: application.BundledAssetFileServer(assets)},
		Windows: application.WindowsOptions{AdditionalBrowserArgs: []string{
			"--disable-background-networking",
			"--disable-component-update",
			"--disable-client-side-phishing-detection",
			"--disable-default-apps",
			"--disable-domain-reliability",
			"--disable-features=MediaRouter,OptimizationHints,AutofillServerCommunication",
			"--disable-sync",
			"--metrics-recording-only",
			"--no-first-run",
		}},
		OnShutdown: func() {
			setInputLocked(false)
			uninstallInputLockHook()
		},
	})
	menu := app.NewMenu()
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                       "main",
		Title:                      "YTLite",
		Width:                      1280,
		Height:                     720,
		MinWidth:                   640,
		MinHeight:                  400,
		DisableResize:              false,
		Frameless:                  true,
		InitialPosition:            application.WindowCentered,
		HTML:                       frontendBootstrapHTML(),
		JS:                         playerGuard + "\n" + channelLinkGuard,
		DefaultContextMenuDisabled: true,
	})
	menu.Add("Show / Hide").OnClick(func(*application.Context) { toggleWindow(window) })
	menu.Add("Quit").OnClick(func(*application.Context) { app.Quit() })
	tray := app.SystemTray.New()
	tray.SetTooltip("YTLite")
	tray.SetIcon(trayIcon).SetMenu(menu)
	tray.OnClick(func() { toggleWindow(window) })
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) { window.Hide(); event.Cancel() })
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
