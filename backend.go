package fynecef

import "fyne.io/fyne/v2/driver/desktop"

type browserBackend interface {
	LoadURL(string) error
	Reload() error
	Stop() error
	GoBack() error
	GoForward() error
	SetBounds(x, y, width, height int) error
	Resize(int, int) error
	SetFrameRate(int) error
	Focus(bool) error
	Close() error

	MouseMove(x, y int, modifiers desktop.Modifier) error
	DragMove(x, y int, buttons desktop.MouseButton, modifiers desktop.Modifier) error
	MouseDown(x, y int, button desktop.MouseButton, modifiers desktop.Modifier) error
	MouseUp(x, y int, button desktop.MouseButton, modifiers desktop.Modifier) error
	MouseWheel(x, y int, deltaX, deltaY int, modifiers desktop.Modifier) error

	KeyDown(name string, modifiers desktop.Modifier) error
	KeyUp(name string, modifiers desktop.Modifier) error
	Rune(r rune) error
}
