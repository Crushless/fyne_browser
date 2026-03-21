package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	fynecef "fyne_browser"
)

func main() {
	if err := fynecef.Init(fynecef.RuntimeOptions{
		FrameworkRoot: filepath.Join("third_party", "cef", "current"),
		SearchPaths: []string{
			filepath.Join("third_party", "cef", "current"),
			filepath.Join("third_party", "cef", "builds"),
		},
		DownloadDir:   filepath.Join("third_party", "cef", "builds"),
		AllowDownload: true,
		ManifestURL:   fynecef.OfficialManifestURL,
		Channel:       fynecef.ChannelStable,
		PackageType:   fynecef.PackageTypeMinimal,
		CachePath:     filepath.Join(os.TempDir(), "fynecef-demo-cache"),
	}); err != nil {
		fmt.Fprintf(os.Stderr, "cef init failed: %v\n", err)
	}

	a := app.New()
	w := a.NewWindow("Fyne CEF Browser")
	w.Resize(fyne.NewSize(1280, 860))

	address := widget.NewEntry()
	address.SetText("https://example.com")
	address.PlaceHolder = "https://example.com"

	status := canvas.NewText("Ready", theme.Color(theme.ColorNameForeground))
	status.TextSize = theme.TextSize()

	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 1

	setStatus := func(text string) {
		status.Text = text
		status.Refresh()
	}

	var browser *fynecef.Browser

	updateNav := func() {
		if browser == nil {
			return
		}
		setStatus(browser.Title())
	}

	refreshAction := widget.NewToolbarAction(theme.ViewRefreshIcon(), func() {
		if err := browser.Reload(); err != nil {
			setStatus(err.Error())
		}
	})
	cancelAction := widget.NewToolbarAction(theme.CancelIcon(), func() {
		if err := browser.Stop(); err != nil {
			setStatus(err.Error())
		}
	})

	browser, browserErr := fynecef.NewBrowser(fynecef.BrowserOptions{
		InitialURL: address.Text,
		Window:     w,
		Callbacks: fynecef.Callbacks{
			OnAddressChange: func(rawURL string) {
				if strings.TrimSpace(rawURL) != "" {
					address.SetText(rawURL)
				}
			},
			OnTitleChange: func(title string) {
				if strings.TrimSpace(title) != "" {
					setStatus(title)
				}
			},
			OnLoadProgress: func(p fynecef.LoadProgress) {
				progress.SetValue(p.Progress)
				if p.IsLoading {
					address.Hide()
					progress.Show()
					refreshAction.Disable()
					cancelAction.Enable()
					return
				}
				progress.Hide()
				address.Show()
				refreshAction.Enable()
				cancelAction.Disable()
				if p.Title != "" {
					setStatus(p.Title)
					return
				}
				if p.URL != "" {
					setStatus(p.URL)
				}
			},
			OnError: func(err error) {
				if err != nil {
					dialog.ShowError(err, w)
				}
				progress.Hide()
				address.Show()
			},
		},
	})
	if browserErr != nil {
		dialog.ShowError(browserErr, w)
	}

	openLocation := func() {
		rawURL := strings.TrimSpace(address.Text)
		if rawURL == "" {
			return
		}
		if !strings.Contains(rawURL, "://") {
			rawURL = "https://" + rawURL
			address.SetText(rawURL)
		}
		if err := browser.LoadURL(rawURL); err != nil {
			setStatus(err.Error())
		}
	}

	address.OnSubmitted = func(string) {
		openLocation()
	}

	toolbar := container.NewBorder(
		nil,
		nil,
		widget.NewToolbar(
			widget.NewToolbarAction(theme.NavigateBackIcon(), func() {
				if err := browser.GoBack(); err != nil {
					setStatus(err.Error())
				}
				updateNav()
			}),
			widget.NewToolbarAction(theme.NavigateNextIcon(), func() {
				if err := browser.GoForward(); err != nil {
					setStatus(err.Error())
				}
				updateNav()
			}),
			refreshAction,
			cancelAction,
			widget.NewToolbarAction(theme.HomeIcon(), func() {
				if err := browser.LoadURL("https://example.com"); err != nil {
					setStatus(err.Error())
				}
				updateNav()
			}),
		),
		widget.NewToolbar(
			widget.NewToolbarAction(theme.MailSendIcon(), openLocation),
		),
		address,
		progress,
	)
	content := container.NewBorder(toolbar, status, nil, nil, browser)
	w.SetContent(content)

	w.SetCloseIntercept(func() {
		_ = browser.Close()
		fynecef.Shutdown()
		w.Close()
	})
	w.ShowAndRun()
}
