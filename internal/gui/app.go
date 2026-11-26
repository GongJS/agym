package gui

import (
	"context"
	"embed"
	"io/fs"
	"runtime"
	"time"

	"github.com/gongjs/agym/internal/accounts"
	"github.com/gongjs/agym/internal/appservice"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Run starts the Wails v3 menu bar tray application using the embedded frontend assets.
func Run(assets embed.FS) error {
	hideConsoleWindow()
	roster, err := accounts.NewDefaultRoster()
	if err != nil {
		return err
	}
	if _, err := roster.RecoverOAuthSlot(context.Background(), accounts.NewOAuthAdder(roster).OwnerFile); err != nil {
		return err
	}
	accountService := appservice.NewAccounts(roster)
	webAssets, err := fs.Sub(assets, "frontend")
	if err != nil {
		return err
	}
	icon, err := assets.ReadFile("frontend/tray-template.png")
	if err != nil {
		return err
	}

	app := application.New(application.Options{
		Name:        "Agy Manager",
		Description: "Manage saved agy accounts from the macOS menu bar",
		Services: []application.Service{
			application.NewService(accountService),
		},
		OnShutdown: accountService.Shutdown,
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(webAssets),
		},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyAccessory,
		},
	})

	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(icon)
	} else {
		tray.SetIcon(icon)
	}
	tray.SetTooltip("Agy Manager")

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:            "account-popup",
		Title:           "Agy Manager",
		URL:             "/",
		Width:           400,
		Height:          600,
		MinWidth:        380,
		MinHeight:       500,
		MaxWidth:        440,
		MaxHeight:       740,
		Frameless:       true,
		AlwaysOnTop:     true,
		Hidden:          true,
		DisableResize:   true,
		HideOnEscape:    true,
		HideOnFocusLost: true,
		BackgroundType:  application.BackgroundTypeTranslucent,
		Mac: application.MacWindow{
			Backdrop:     application.MacBackdropTranslucent,
			CornerRadius: 16,
		},
	})
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		window.Hide()
		event.Cancel()
	})
	tray.AttachWindow(window).WindowOffset(6).WindowDebounce(250 * time.Millisecond)

	menu := app.NewMenu()
	menu.Add("Open Account Manager").OnClick(func(_ *application.Context) {
		window.Show()
	})
	menu.AddSeparator()
	menu.Add("Quit Agy Manager").OnClick(func(_ *application.Context) {
		app.Quit()
	})
	tray.SetMenu(menu)

	return app.Run()
}
