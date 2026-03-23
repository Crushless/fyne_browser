//go:build cgo && darwin

package fynecef

/*
#cgo CFLAGS: -fblocks
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>
#include "webkit_darwin.h"
*/
import "C"

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/driver/desktop"
)

var (
	registrySeq atomic.Uint64
	browserByID sync.Map
)

type darwinBrowserBackend struct {
	browser *Browser
	window  fyne.Window

	callbackID uintptr

	mu         sync.Mutex
	native     *C.fynecef_browser_t
	parent     uintptr
	x          int
	y          int
	width      int
	height     int
	pendingURL string
	closed     bool
}

type browserSnapshot struct {
	url          string
	title        string
	progress     float64
	isLoading    bool
	canGoBack    bool
	canGoForward bool
}

func MaybeRunSubprocess() (bool, int) {
	return false, 0
}

func Init(RuntimeOptions) error {
	return nil
}

func Shutdown() {
}

func newBrowserBackend(browser *Browser, opts BrowserOptions) (browserBackend, error) {
	if opts.Window == nil {
		return nil, ErrWindowRequired
	}

	id := uintptr(registrySeq.Add(1))
	backend := &darwinBrowserBackend{
		browser:    browser,
		window:     opts.Window,
		callbackID: id,
	}
	browserByID.Store(id, backend)
	return backend, nil
}

func (b *darwinBrowserBackend) LoadURL(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.pendingURL = rawURL
	if err := b.ensureCreatedLocked(); err != nil {
		return err
	}
	if b.native == nil {
		return nil
	}

	url := C.CString(rawURL)
	defer C.free(unsafe.Pointer(url))
	C.fynecef_browser_load_url(b.native, url)
	return nil
}

func (b *darwinBrowserBackend) Reload() error {
	return b.withNative(func(native *C.fynecef_browser_t) {
		C.fynecef_browser_reload(native)
	})
}

func (b *darwinBrowserBackend) Stop() error {
	return b.withNative(func(native *C.fynecef_browser_t) {
		C.fynecef_browser_stop(native)
	})
}

func (b *darwinBrowserBackend) GoBack() error {
	return b.withNative(func(native *C.fynecef_browser_t) {
		C.fynecef_browser_go_back(native)
	})
}

func (b *darwinBrowserBackend) GoForward() error {
	return b.withNative(func(native *C.fynecef_browser_t) {
		C.fynecef_browser_go_forward(native)
	})
}

func (b *darwinBrowserBackend) SetBounds(x, y, width, height int) error {
	b.mu.Lock()
	if b.native != nil && x == b.x && y == b.y && width == b.width && height == b.height {
		b.mu.Unlock()
		return nil
	}

	b.x = x
	b.y = y
	b.width = width
	b.height = height

	if err := b.ensureCreatedLocked(); err != nil {
		b.mu.Unlock()
		return err
	}
	if b.native == nil {
		b.mu.Unlock()
		return nil
	}

	native := b.native
	b.mu.Unlock()
	C.fynecef_browser_set_bounds(native, C.int(x), C.int(y), C.int(width), C.int(height))
	return nil
}

func (b *darwinBrowserBackend) Resize(width, height int) error {
	b.mu.Lock()
	x, y := b.x, b.y
	b.mu.Unlock()
	return b.SetBounds(x, y, width, height)
}

func (b *darwinBrowserBackend) SetFrameRate(int) error {
	return nil
}

func (b *darwinBrowserBackend) Focus(focus bool) error {
	var value C.int
	if focus {
		value = 1
	}
	return b.withNative(func(native *C.fynecef_browser_t) {
		C.fynecef_browser_set_focus(native, value)
	})
}

func (b *darwinBrowserBackend) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	native := b.native
	b.mu.Unlock()

	if native == nil {
		browserByID.Delete(b.callbackID)
		return nil
	}

	C.fynecef_browser_close(native)
	return nil
}

func (b *darwinBrowserBackend) MouseMove(int, int, desktop.Modifier) error {
	return nil
}

func (b *darwinBrowserBackend) DragMove(int, int, desktop.MouseButton, desktop.Modifier) error {
	return nil
}

func (b *darwinBrowserBackend) MouseDown(int, int, desktop.MouseButton, desktop.Modifier) error {
	return nil
}

func (b *darwinBrowserBackend) MouseUp(int, int, desktop.MouseButton, desktop.Modifier) error {
	return nil
}

func (b *darwinBrowserBackend) MouseWheel(int, int, int, int, desktop.Modifier) error {
	return nil
}

func (b *darwinBrowserBackend) KeyDown(string, desktop.Modifier) error {
	return nil
}

func (b *darwinBrowserBackend) KeyUp(string, desktop.Modifier) error {
	return nil
}

func (b *darwinBrowserBackend) Rune(rune) error {
	return nil
}

func (b *darwinBrowserBackend) withNative(fn func(*C.fynecef_browser_t)) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.ensureCreatedLocked(); err != nil {
		return err
	}
	if b.native == nil {
		return nil
	}

	fn(b.native)
	return nil
}

func (b *darwinBrowserBackend) ensureCreatedLocked() error {
	if b.closed || b.native != nil || b.width <= 0 || b.height <= 0 {
		return nil
	}
	if b.parent == 0 {
		handle, err := nativeParentHandle(b.window)
		if err != nil {
			return err
		}
		if handle == 0 {
			return nil
		}
		b.parent = handle
	}

	initialURL := strings.TrimSpace(b.pendingURL)
	if initialURL == "" {
		initialURL = "about:blank"
	}
	url := C.CString(initialURL)
	defer C.free(unsafe.Pointer(url))

	native := C.fynecef_browser_create(
		C.uintptr_t(b.callbackID),
		C.uintptr_t(b.parent),
		C.int(b.x),
		C.int(b.y),
		C.int(b.width),
		C.int(b.height),
		url,
	)
	if native == nil {
		return fmt.Errorf("%w: create browser returned nil", ErrRuntimeInit)
	}

	b.native = native
	return nil
}

func nativeParentHandle(window fyne.Window) (uintptr, error) {
	nativeWindow, ok := window.(driver.NativeWindow)
	if !ok {
		return 0, ErrPlatformUnsupported
	}

	var (
		handle uintptr
		err    error
	)
	nativeWindow.RunNative(func(ctx any) {
		switch value := ctx.(type) {
		case driver.MacWindowContext:
			handle = value.NSWindow
		default:
			err = ErrPlatformUnsupported
		}
	})
	return handle, err
}

func lookupBackend(id uintptr) *darwinBrowserBackend {
	value, ok := browserByID.Load(id)
	if !ok {
		return nil
	}
	backend, _ := value.(*darwinBrowserBackend)
	return backend
}

func snapshot(browser *Browser) browserSnapshot {
	browser.mu.RLock()
	defer browser.mu.RUnlock()
	return browserSnapshot{
		url:          browser.currentURL,
		title:        browser.currentTitle,
		progress:     browser.loadingPct,
		isLoading:    browser.loading,
		canGoBack:    browser.canGoBack,
		canGoForward: browser.canGoForward,
	}
}

func resourceTypeName(value int) string {
	switch value {
	case 0:
		return "main_frame"
	default:
		return fmt.Sprintf("resource_%d", value)
	}
}

//export goCEFOnAddressChange
func goCEFOnAddressChange(handle C.uintptr_t, url *C.char) {
	backend := lookupBackend(uintptr(handle))
	if backend == nil {
		return
	}
	backend.browser.setAddress(C.GoString(url))
}

//export goCEFOnTitleChange
func goCEFOnTitleChange(handle C.uintptr_t, title *C.char) {
	backend := lookupBackend(uintptr(handle))
	if backend == nil {
		return
	}
	backend.browser.setTitle(C.GoString(title))
}

//export goCEFOnLoadProgress
func goCEFOnLoadProgress(handle C.uintptr_t, progress C.double) {
	backend := lookupBackend(uintptr(handle))
	if backend == nil {
		return
	}
	state := snapshot(backend.browser)
	backend.browser.emitProgress(LoadProgress{
		URL:          state.url,
		Title:        state.title,
		Progress:     float64(progress),
		IsLoading:    state.isLoading,
		CanGoBack:    state.canGoBack,
		CanGoForward: state.canGoForward,
	})
}

//export goCEFOnLoadingStateChange
func goCEFOnLoadingStateChange(handle C.uintptr_t, isLoading C.int, canGoBack C.int, canGoForward C.int) {
	backend := lookupBackend(uintptr(handle))
	if backend == nil {
		return
	}
	state := snapshot(backend.browser)
	backend.browser.emitProgress(LoadProgress{
		URL:          state.url,
		Title:        state.title,
		Progress:     state.progress,
		IsLoading:    isLoading != 0,
		CanGoBack:    canGoBack != 0,
		CanGoForward: canGoForward != 0,
	})
}

//export goCEFOnLoadError
func goCEFOnLoadError(handle C.uintptr_t, code C.int, errorText *C.char, failedURL *C.char) {
	backend := lookupBackend(uintptr(handle))
	if backend == nil {
		return
	}
	text := C.GoString(errorText)
	url := C.GoString(failedURL)
	backend.browser.emitError(fmt.Errorf("fynecef: load error %d: %s (%s)", int(code), text, url))
}

//export goCEFOnBeforeResourceLoad
func goCEFOnBeforeResourceLoad(handle C.uintptr_t, url *C.char, method *C.char, initiator *C.char, resourceType C.int, isNavigation C.int) C.int {
	backend := lookupBackend(uintptr(handle))
	if backend == nil {
		return 0
	}
	callback := backend.browser.options.Callbacks.OnBeforeResourceLoad
	if callback == nil {
		return 0
	}
	decision := callback(ResourceRequest{
		URL:          C.GoString(url),
		Method:       C.GoString(method),
		Initiator:    C.GoString(initiator),
		ResourceType: resourceTypeName(int(resourceType)),
		IsNavigation: isNavigation != 0,
	})
	if decision == ResourceBlock {
		return 1
	}
	return 0
}

//export goCEFOnBeforeClose
func goCEFOnBeforeClose(handle C.uintptr_t) {
	backend := lookupBackend(uintptr(handle))
	if backend == nil {
		return
	}
	backend.mu.Lock()
	backend.native = nil
	backend.closed = true
	backend.mu.Unlock()
	browserByID.Delete(uintptr(handle))
}
