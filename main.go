package main

import (
	"log"

	webview "github.com/samcharles93/webview_go"
)

func main() {
	app := NewDesktopApp()

	serverURL, err := app.StartServer()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = app.Shutdown()
	}()

	window := webview.New(false)
	if window == nil {
		log.Fatal("failed to create webview window")
	}
	defer window.Destroy()

	app.SetWebView(window)

	mustBind(window, "getAppVersion", app.GetAppVersion)
	mustBind(window, "selectDirectories", app.SelectDirectories)
	mustBind(window, "startScanDirectories", app.StartScanDirectories)
	mustBind(window, "getScanStatus", app.GetScanStatus)
	mustBind(window, "startDeleteFiles", app.StartDeleteFiles)
	mustBind(window, "getDeleteStatus", app.GetDeleteStatus)
	mustBind(window, "revealFile", app.RevealFile)
	mustBind(window, "previewFile", app.PreviewFile)

	window.SetTitle(appName + " v" + appVersion)
	window.SetSize(980, 720, webview.HintMin)
	window.SetSize(1240, 860, webview.HintNone)
	window.Init(app.BridgeScript())
	window.Navigate(serverURL)
	window.Run()
}

func mustBind(window webview.WebView, name string, target interface{}) {
	if err := window.Bind(name, target); err != nil {
		log.Fatalf("bind %s failed: %v", name, err)
	}
}
