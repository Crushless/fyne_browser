//go:build cgo && windows

package fynecef

import "fyne.io/fyne/v2/driver/desktop"

type unsupportedBackend struct{}

func newBrowserBackend(*Browser, BrowserOptions) (browserBackend, error) {
	return &unsupportedBackend{}, ErrPlatformUnsupported
}

func (unsupportedBackend) LoadURL(string) error { return ErrPlatformUnsupported }

func (unsupportedBackend) Reload() error { return ErrPlatformUnsupported }

func (unsupportedBackend) Stop() error { return ErrPlatformUnsupported }

func (unsupportedBackend) GoBack() error { return ErrPlatformUnsupported }

func (unsupportedBackend) GoForward() error { return ErrPlatformUnsupported }

func (unsupportedBackend) SetBounds(int, int, int, int) error { return ErrPlatformUnsupported }

func (unsupportedBackend) Resize(int, int) error { return ErrPlatformUnsupported }

func (unsupportedBackend) SetFrameRate(int) error { return ErrPlatformUnsupported }

func (unsupportedBackend) Focus(bool) error { return ErrPlatformUnsupported }

func (unsupportedBackend) Close() error { return nil }

func (unsupportedBackend) MouseMove(int, int, desktop.Modifier) error { return ErrPlatformUnsupported }

func (unsupportedBackend) DragMove(int, int, desktop.MouseButton, desktop.Modifier) error {
	return ErrPlatformUnsupported
}

func (unsupportedBackend) MouseDown(int, int, desktop.MouseButton, desktop.Modifier) error {
	return ErrPlatformUnsupported
}

func (unsupportedBackend) MouseUp(int, int, desktop.MouseButton, desktop.Modifier) error {
	return ErrPlatformUnsupported
}

func (unsupportedBackend) MouseWheel(int, int, int, int, desktop.Modifier) error {
	return ErrPlatformUnsupported
}

func (unsupportedBackend) KeyDown(string, desktop.Modifier) error { return ErrPlatformUnsupported }

func (unsupportedBackend) KeyUp(string, desktop.Modifier) error { return ErrPlatformUnsupported }

func (unsupportedBackend) Rune(rune) error { return ErrPlatformUnsupported }
