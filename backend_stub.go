//go:build !cgo || (!linux && !darwin && !windows)

package fynecef

import (
	"fmt"

	"fyne.io/fyne/v2/driver/desktop"
)

type stubBackend struct {
	browser *Browser
}

func newBrowserBackend(browser *Browser, _ BrowserOptions) (browserBackend, error) {
	return &stubBackend{browser: browser}, ErrCEFNotBuilt
}

func (s *stubBackend) LoadURL(rawURL string) error {
	s.browser.setAddress(rawURL)
	s.browser.emitProgress(LoadProgress{
		URL:          rawURL,
		Title:        s.browser.Title(),
		Progress:     0,
		IsLoading:    false,
		CanGoBack:    false,
		CanGoForward: false,
	})
	s.browser.emitError(fmt.Errorf("%w: run the bootstrapper to fetch the SDK and add a native CEF backend build", ErrCEFNotBuilt))
	return ErrCEFNotBuilt
}

func (s *stubBackend) Reload() error { return ErrCEFNotBuilt }

func (s *stubBackend) Stop() error { return ErrCEFNotBuilt }

func (s *stubBackend) GoBack() error { return ErrCEFNotBuilt }

func (s *stubBackend) GoForward() error { return ErrCEFNotBuilt }

func (s *stubBackend) SetBounds(int, int, int, int) error { return nil }

func (s *stubBackend) Resize(int, int) error { return nil }

func (s *stubBackend) SetFrameRate(int) error { return nil }

func (s *stubBackend) Focus(bool) error { return nil }

func (s *stubBackend) Close() error { return nil }

func (s *stubBackend) MouseMove(int, int, desktop.Modifier) error { return ErrCEFNotBuilt }

func (s *stubBackend) DragMove(int, int, desktop.MouseButton, desktop.Modifier) error {
	return ErrCEFNotBuilt
}

func (s *stubBackend) MouseDown(int, int, desktop.MouseButton, desktop.Modifier, int) error {
	return ErrCEFNotBuilt
}

func (s *stubBackend) MouseUp(int, int, desktop.MouseButton, desktop.Modifier, int) error {
	return ErrCEFNotBuilt
}

func (s *stubBackend) MouseWheel(int, int, int, int, desktop.Modifier) error { return ErrCEFNotBuilt }

func (s *stubBackend) KeyDown(string, desktop.Modifier) error { return ErrCEFNotBuilt }

func (s *stubBackend) KeyUp(string, desktop.Modifier) error { return ErrCEFNotBuilt }

func (s *stubBackend) Rune(rune) error { return ErrCEFNotBuilt }
