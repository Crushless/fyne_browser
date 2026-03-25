//go:build cgo && linux

package fynecef

/*
#cgo CFLAGS: -I${SRCDIR}/third_party/cef/current
#cgo LDFLAGS: -L${SRCDIR}/third_party/cef/current/Release -Wl,-rpath,${SRCDIR}/third_party/cef/current/Release -lcef -lX11
#include <stdlib.h>
#include "cef_linux.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

const (
	cefCursorPointer          = 0
	cefCursorCross            = 1
	cefCursorHand             = 2
	cefCursorIBeam            = 3
	cefCursorEastResize       = 6
	cefCursorNorthResize      = 7
	cefCursorSouthResize      = 10
	cefCursorWestResize       = 13
	cefCursorNorthSouthResize = 14
	cefCursorEastWestResize   = 15
	cefCursorColumnResize     = 18
	cefCursorRowResize        = 19
	cefCursorVerticalText     = 30
	cefCursorCell             = 31
	cefCursorNone             = 37
	cefCursorCustom           = 45
	cefMenuItemTypeNone       = 0
	cefMenuItemTypeCommand    = 1
	cefMenuItemTypeCheck      = 2
	cefMenuItemTypeRadio      = 3
	cefMenuItemTypeSeparator  = 4
	cefMenuItemTypeSubmenu    = 5
	cefMenuIDFind             = 130
	cefMenuIDPrint            = 131
	cefMenuIDViewSource       = 132
)

const frameworkRootEnv = "FYNECEF_FRAMEWORK_ROOT"
const subprocessPathEnv = "FYNECEF_SUBPROCESS_PATH"

var (
	runtimeMu    sync.Mutex
	runtimeState *cefRuntime
	registrySeq  atomic.Uint64
	browserByID  sync.Map
)

type cefRuntime struct {
	framework *Framework
	thread    *cefThread
}

type linuxRuntimeLayout struct {
	frameworkRoot string
	moduleDir     string
	resourcesDir  string
	localesDir    string
}

type cefThread struct {
	tasks chan func()
	stop  chan chan struct{}
}

type linuxBrowserBackend struct {
	browser *Browser
	window  fyne.Window
	runtime *cefRuntime

	callbackID uintptr

	mu          sync.Mutex
	native      *C.fynecef_browser_t
	parent      uintptr
	x           int
	y           int
	width       int
	height      int
	pendingURL  string
	closed      bool
	contextMenu *contextMenuSession
}

type contextMenuSession struct {
	native *C.fynecef_context_menu_t
	popup  *widget.PopUpMenu
	done   atomic.Bool
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
	framework, err := resolveRuntimeFramework(RuntimeOptions{
		FrameworkRoot: os.Getenv(frameworkRootEnv),
	}, false)
	if err != nil || framework == nil {
		return false, 0
	}
	layout, err := prepareLinuxRuntimeLayout(framework.Root)
	if err != nil {
		return false, 0
	}
	executable, err := os.Executable()
	if err != nil {
		return false, 0
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return false, 0
	}
	subprocessPath := strings.TrimSpace(os.Getenv(subprocessPathEnv))
	if subprocessPath == "" {
		subprocessPath = executable
	}

	argv, freeFn := makeCStringSlice(cefCommandLineArgs(os.Args, subprocessPath, layout))
	defer freeFn()

	code := int(C.fynecef_execute_process(C.int(len(argv)), argvPointer(argv)))
	if code >= 0 {
		return true, code
	}
	return false, 0
}

func Init(opts RuntimeOptions) error {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()

	if runtimeState != nil {
		return nil
	}

	framework, err := resolveRuntimeFramework(opts, true)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRuntimeInit, err)
	}
	layout, err := prepareLinuxRuntimeLayout(framework.Root)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRuntimeInit, err)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("%w: locate executable: %v", ErrRuntimeInit, err)
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return fmt.Errorf("%w: normalize executable path: %v", ErrRuntimeInit, err)
	}

	cachePath := opts.CachePath
	if strings.TrimSpace(cachePath) == "" {
		cachePath = defaultCachePath()
	}
	cachePath, err = filepath.Abs(cachePath)
	if err != nil {
		return fmt.Errorf("%w: normalize cache dir: %v", ErrRuntimeInit, err)
	}
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		return fmt.Errorf("%w: create cache dir: %v", ErrRuntimeInit, err)
	}
	subprocessPath, err := prepareSubprocessExecutable(exe, layout.moduleDir)
	if err != nil {
		return fmt.Errorf("%w: prepare subprocess executable: %v", ErrRuntimeInit, err)
	}

	if err := os.Setenv(frameworkRootEnv, framework.Root); err != nil {
		return fmt.Errorf("%w: set framework env: %v", ErrRuntimeInit, err)
	}
	if err := os.Setenv(subprocessPathEnv, subprocessPath); err != nil {
		return fmt.Errorf("%w: set subprocess env: %v", ErrRuntimeInit, err)
	}

	thread, err := startCEFThread(subprocessPath, layout, cachePath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRuntimeInit, err)
	}

	runtimeState = &cefRuntime{
		framework: framework,
		thread:    thread,
	}
	return nil
}

func Shutdown() {
	runtimeMu.Lock()
	rt := runtimeState
	runtimeState = nil
	runtimeMu.Unlock()

	if rt == nil || rt.thread == nil {
		return
	}
	rt.thread.shutdown()
}

func newBrowserBackend(browser *Browser, opts BrowserOptions) (browserBackend, error) {
	if opts.Window == nil {
		return nil, ErrWindowRequired
	}

	rt, err := ensureRuntime(RuntimeOptions{})
	if err != nil {
		return nil, err
	}

	id := uintptr(registrySeq.Add(1))
	backend := &linuxBrowserBackend{
		browser:    browser,
		window:     opts.Window,
		runtime:    rt,
		callbackID: id,
	}
	browserByID.Store(id, backend)
	return backend, nil
}

func (b *linuxBrowserBackend) LoadURL(rawURL string) error {
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

	native := b.native
	url := C.CString(rawURL)
	defer C.free(unsafe.Pointer(url))

	return b.runtime.thread.run(func() error {
		C.fynecef_browser_load_url(native, url)
		return nil
	})
}

func (b *linuxBrowserBackend) Reload() error {
	return b.withNative(func(native *C.fynecef_browser_t) {
		C.fynecef_browser_reload(native)
	})
}

func (b *linuxBrowserBackend) Stop() error {
	return b.withNative(func(native *C.fynecef_browser_t) {
		C.fynecef_browser_stop(native)
	})
}

func (b *linuxBrowserBackend) GoBack() error {
	return b.withNative(func(native *C.fynecef_browser_t) {
		C.fynecef_browser_go_back(native)
	})
}

func (b *linuxBrowserBackend) GoForward() error {
	return b.withNative(func(native *C.fynecef_browser_t) {
		C.fynecef_browser_go_forward(native)
	})
}

func (b *linuxBrowserBackend) SetBounds(x, y, width, height int) error {
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
	return b.runtime.thread.run(func() error {
		C.fynecef_browser_set_bounds(native, C.int(x), C.int(y), C.int(width), C.int(height))
		return nil
	})
}

func (b *linuxBrowserBackend) Resize(width, height int) error {
	b.mu.Lock()
	x, y := b.x, b.y
	b.mu.Unlock()
	return b.SetBounds(x, y, width, height)
}

func (b *linuxBrowserBackend) SetFrameRate(rate int) error {
	if rate < 1 {
		rate = 1
	} else if rate > 60 {
		rate = 60
	}
	return b.withNative(func(native *C.fynecef_browser_t) {
		C.fynecef_browser_set_windowless_frame_rate(native, C.int(rate))
	})
}

func (b *linuxBrowserBackend) Focus(focus bool) error {
	var value C.int
	if focus {
		value = 1
	}
	return b.withNative(func(native *C.fynecef_browser_t) {
		C.fynecef_browser_set_focus(native, value)
	})
}

func (b *linuxBrowserBackend) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	native := b.native
	contextMenu := b.contextMenu
	b.contextMenu = nil
	b.mu.Unlock()

	if contextMenu != nil {
		dispatchToFyne(func() {
			b.cancelAndHideContextMenu(contextMenu)
		})
	}

	if native == nil {
		browserByID.Delete(b.callbackID)
		return nil
	}

	return b.runtime.thread.run(func() error {
		C.fynecef_browser_close(native)
		return nil
	})
}

func (b *linuxBrowserBackend) MouseMove(x, y int, modifiers desktop.Modifier) error {
	return b.withNative(func(native *C.fynecef_browser_t) {
		C.fynecef_browser_mouse_move(native, C.int(x), C.int(y), cefModifiers(modifiers), 0)
	})
}

func (b *linuxBrowserBackend) DragMove(x, y int, buttons desktop.MouseButton, modifiers desktop.Modifier) error {
	return b.withNative(func(native *C.fynecef_browser_t) {
		value := cefModifiers(modifiers) | cefMouseButtonsModifier(buttons)
		C.fynecef_browser_mouse_move(native, C.int(x), C.int(y), value, 0)
	})
}

func (b *linuxBrowserBackend) MouseDown(x, y int, button desktop.MouseButton, modifiers desktop.Modifier, clickCount int) error {
	return b.withNative(func(native *C.fynecef_browser_t) {
		C.fynecef_browser_mouse_click(native, C.int(x), C.int(y), cefModifiers(modifiers), cefMouseButton(button), 0, C.int(clickCount))
	})
}

func (b *linuxBrowserBackend) MouseUp(x, y int, button desktop.MouseButton, modifiers desktop.Modifier, clickCount int) error {
	return b.withNative(func(native *C.fynecef_browser_t) {
		C.fynecef_browser_mouse_click(native, C.int(x), C.int(y), cefModifiers(modifiers), cefMouseButton(button), 1, C.int(clickCount))
	})
}

func (b *linuxBrowserBackend) MouseWheel(x, y int, deltaX, deltaY int, modifiers desktop.Modifier) error {
	return b.withNative(func(native *C.fynecef_browser_t) {
		C.fynecef_browser_mouse_wheel(native, C.int(x), C.int(y), cefModifiers(modifiers), C.int(deltaX), C.int(deltaY))
	})
}

func (b *linuxBrowserBackend) KeyDown(name string, modifiers desktop.Modifier) error {
	key, ok := cefKeyCode(name)
	if !ok {
		return nil
	}
	return b.withNative(func(native *C.fynecef_browser_t) {
		C.fynecef_browser_key_event(native, C.int(C.KEYEVENT_RAWKEYDOWN), cefModifiers(modifiers), C.int(key), C.int(key), 0, 0)
		C.fynecef_browser_key_event(native, C.int(C.KEYEVENT_KEYDOWN), cefModifiers(modifiers), C.int(key), C.int(key), 0, 0)
	})
}

func (b *linuxBrowserBackend) KeyUp(name string, modifiers desktop.Modifier) error {
	key, ok := cefKeyCode(name)
	if !ok {
		return nil
	}
	return b.withNative(func(native *C.fynecef_browser_t) {
		C.fynecef_browser_key_event(native, C.int(C.KEYEVENT_KEYUP), cefModifiers(modifiers), C.int(key), C.int(key), 0, 0)
	})
}

func (b *linuxBrowserBackend) Rune(value rune) error {
	if value == 0 {
		return nil
	}
	return b.withNative(func(native *C.fynecef_browser_t) {
		ch := C.uint16_t(value)
		C.fynecef_browser_key_event(native, C.int(C.KEYEVENT_CHAR), 0, C.int(value), C.int(value), ch, ch)
	})
}

func (b *linuxBrowserBackend) withNative(fn func(*C.fynecef_browser_t)) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.ensureCreatedLocked(); err != nil {
		return err
	}
	if b.native == nil {
		return nil
	}

	native := b.native
	return b.runtime.thread.run(func() error {
		fn(native)
		return nil
	})
}

func (b *linuxBrowserBackend) ensureCreatedLocked() error {
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

	url := C.CString("about:blank")
	defer C.free(unsafe.Pointer(url))

	var native *C.fynecef_browser_t
	if err := b.runtime.thread.run(func() error {
		native = C.fynecef_browser_create(
			C.uintptr_t(b.callbackID),
			C.uintptr_t(b.parent),
			C.int(b.x),
			C.int(b.y),
			C.int(b.width),
			C.int(b.height),
			url,
		)
		if native == nil {
			return errors.New("create browser returned nil")
		}
		return nil
	}); err != nil {
		return err
	}

	b.native = native
	if pending := strings.TrimSpace(b.pendingURL); pending != "" && !strings.EqualFold(pending, "about:blank") {
		loadURL := C.CString(pending)
		defer C.free(unsafe.Pointer(loadURL))
		if err := b.runtime.thread.run(func() error {
			C.fynecef_browser_load_url(native, loadURL)
			return nil
		}); err != nil {
			return err
		}
	}
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
		case driver.X11WindowContext:
			handle = value.WindowHandle
		case driver.WaylandWindowContext:
			err = fmt.Errorf("%w: Linux embedding currently requires X11", ErrPlatformUnsupported)
		default:
			err = ErrPlatformUnsupported
		}
	})
	return handle, err
}

func ensureRuntime(opts RuntimeOptions) (*cefRuntime, error) {
	runtimeMu.Lock()
	current := runtimeState
	runtimeMu.Unlock()
	if current != nil {
		return current, nil
	}
	if err := Init(opts); err != nil {
		return nil, err
	}
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	return runtimeState, nil
}

func resolveRuntimeFramework(opts RuntimeOptions, allowDownload bool) (*Framework, error) {
	searchPaths := append([]string{}, opts.SearchPaths...)
	if opts.FrameworkRoot != "" {
		searchPaths = append([]string{opts.FrameworkRoot}, searchPaths...)
	}
	searchPaths = append(searchPaths, defaultSearchPaths()...)
	searchPaths = dedupeStrings(searchPaths)

	destination := opts.DownloadDir
	if strings.TrimSpace(destination) == "" {
		destination = defaultDownloadDir()
	}

	channel := opts.Channel
	if channel == "" {
		channel = ChannelStable
	}
	packageType := opts.PackageType
	if packageType == "" {
		packageType = PackageTypeMinimal
	}

	return EnsureFramework(InstallOptions{
		ManifestURL:   opts.ManifestURL,
		Channel:       channel,
		PackageType:   packageType,
		Destination:   destination,
		SearchPaths:   searchPaths,
		AllowDownload: allowDownload || opts.AllowDownload,
	})
}

func defaultSearchPaths() []string {
	paths := []string{
		filepath.Join("third_party", "cef", "current"),
		filepath.Join("third_party", "cef", "builds"),
	}
	if envRoot := strings.TrimSpace(os.Getenv(frameworkRootEnv)); envRoot != "" {
		paths = append([]string{envRoot}, paths...)
	}
	paths = append(paths, defaultDownloadDir())
	return dedupeStrings(paths)
}

func defaultDownloadDir() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "fynecef-builds")
	}
	return filepath.Join(cacheDir, "fynecef", "builds")
}

func defaultCachePath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "fynecef-profile")
	}
	return filepath.Join(cacheDir, "fynecef", "profile")
}

func prepareLinuxRuntimeLayout(frameworkRoot string) (linuxRuntimeLayout, error) {
	root, err := filepath.Abs(frameworkRoot)
	if err != nil {
		return linuxRuntimeLayout{}, err
	}

	releaseDir := filepath.Join(root, "Release")
	resourceDir := filepath.Join(root, "Resources")
	localesSourceDir := filepath.Join(resourceDir, "locales")
	localesTargetDir := filepath.Join(releaseDir, "locales")

	requiredFiles := []string{
		"chrome_100_percent.pak",
		"chrome_200_percent.pak",
		"resources.pak",
		"icudtl.dat",
	}
	for _, name := range requiredFiles {
		if err := ensureLinkedOrCopiedFile(
			filepath.Join(resourceDir, name),
			filepath.Join(releaseDir, name),
		); err != nil {
			return linuxRuntimeLayout{}, err
		}
	}
	if err := ensureLinkedOrCopiedDir(localesSourceDir, localesTargetDir); err != nil {
		return linuxRuntimeLayout{}, err
	}

	return linuxRuntimeLayout{
		frameworkRoot: root,
		moduleDir:     releaseDir,
		resourcesDir:  releaseDir,
		localesDir:    localesTargetDir,
	}, nil
}

func ensureLinkedOrCopiedFile(source, target string) error {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return err
	}
	if sourceInfo.IsDir() {
		return fmt.Errorf("%s is a directory", source)
	}

	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, resolveErr := filepath.EvalSymlinks(target)
			if resolveErr == nil {
				resolved, _ = filepath.Abs(resolved)
				absSource, _ := filepath.Abs(source)
				if resolved == absSource {
					return nil
				}
			}
		}
		if info.Mode().IsRegular() {
			return nil
		}
		if err := os.RemoveAll(target); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	relSource, relErr := filepath.Rel(filepath.Dir(target), source)
	if relErr == nil {
		if err := os.Symlink(relSource, target); err == nil {
			return nil
		}
	}
	return copyFile(source, target, sourceInfo.Mode())
}

func ensureLinkedOrCopiedDir(source, target string) error {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !sourceInfo.IsDir() {
		return fmt.Errorf("%s is not a directory", source)
	}

	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
			return nil
		}
		if err := os.RemoveAll(target); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	relSource, relErr := filepath.Rel(filepath.Dir(target), source)
	if relErr == nil {
		if err := os.Symlink(relSource, target); err == nil {
			return nil
		}
	}
	return copyDir(source, target)
}

func copyFile(source, target string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func copyDir(source, target string) error {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(source, entry.Name())
		dstPath := filepath.Join(target, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := copyFile(srcPath, dstPath, info.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func prepareSubprocessExecutable(sourceExecutable, targetDir string) (string, error) {
	if isStableExecutable(sourceExecutable) {
		return sourceExecutable, nil
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", err
	}
	helperPath := filepath.Join(targetDir, "fynecef-subprocess")

	sourceInfo, err := os.Stat(sourceExecutable)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(helperPath); err == nil {
		if info.Size() == sourceInfo.Size() && !sourceInfo.ModTime().After(info.ModTime()) {
			return helperPath, nil
		}
	}
	if err := copyFile(sourceExecutable, helperPath, sourceInfo.Mode()); err != nil {
		return "", err
	}
	if err := os.Chmod(helperPath, 0o755); err != nil {
		return "", err
	}
	return helperPath, nil
}

func isStableExecutable(path string) bool {
	path = filepath.Clean(path)
	tempDir := filepath.Clean(os.TempDir())
	if !strings.HasPrefix(path, tempDir+string(os.PathSeparator)) {
		return true
	}
	base := filepath.Base(path)
	if strings.HasPrefix(base, "go-build") || strings.HasPrefix(path, filepath.Join(tempDir, "go-build")) {
		return false
	}
	return false
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		clean := filepath.Clean(value)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		result = append(result, clean)
	}
	return result
}

func startCEFThread(subprocessPath string, layout linuxRuntimeLayout, cachePath string) (*cefThread, error) {
	thread := &cefThread{
		tasks: make(chan func()),
		stop:  make(chan chan struct{}),
	}

	ready := make(chan error, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		argv, freeFn := makeCStringSlice(cefCommandLineArgs(os.Args, subprocessPath, layout))
		defer freeFn()

		executableC := C.CString(subprocessPath)
		resourcesC := C.CString(layout.resourcesDir)
		localesC := C.CString(layout.localesDir)
		cacheC := C.CString(cachePath)
		defer C.free(unsafe.Pointer(executableC))
		defer C.free(unsafe.Pointer(resourcesC))
		defer C.free(unsafe.Pointer(localesC))
		defer C.free(unsafe.Pointer(cacheC))

		ok := C.fynecef_initialize(
			C.int(len(argv)),
			argvPointer(argv),
			executableC,
			resourcesC,
			localesC,
			cacheC,
		)
		if ok == 0 {
			ready <- errors.New("cef_initialize failed")
			return
		}

		ready <- nil

		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case task := <-thread.tasks:
				task()
			case stop := <-thread.stop:
				C.fynecef_shutdown()
				close(stop)
				return
			case <-ticker.C:
				C.fynecef_do_message_loop_work()
			}
		}
	}()

	if err := <-ready; err != nil {
		return nil, err
	}
	return thread, nil
}

func (t *cefThread) run(fn func() error) error {
	done := make(chan error, 1)
	t.tasks <- func() {
		done <- fn()
	}
	return <-done
}

func (t *cefThread) shutdown() {
	done := make(chan struct{})
	t.stop <- done
	<-done
}

func makeCStringSlice(args []string) ([]*C.char, func()) {
	argv := make([]*C.char, len(args))
	for i, arg := range args {
		argv[i] = C.CString(arg)
	}
	return argv, func() {
		for _, arg := range argv {
			C.free(unsafe.Pointer(arg))
		}
	}
}

func cefCommandLineArgs(base []string, executable string, layout linuxRuntimeLayout) []string {
	args := append([]string{}, base...)
	args = ensureSwitch(args, "--browser-subprocess-path="+executable)
	args = ensureSwitch(args, "--resources-dir-path="+layout.resourcesDir)
	args = ensureSwitch(args, "--locales-dir-path="+layout.localesDir)
	//args = ensureSwitch(args, "--disable-background-networking")
	args = ensureSwitch(args, "--disable-component-update")
	args = ensureSwitch(args, "--disable-default-apps")
	//args = ensureSwitch(args, "--disable-domain-reliability")
	//args = ensureSwitch(args, "--no-sandbox")
	args = ensureSwitch(args, "--no-default-browser-check")
	args = ensureSwitch(args, "--no-zygote")
	//args = ensureSwitch(args, "--disable-gpu")
	//args = ensureSwitch(args, "--disable-gpu-compositing")
	//args = ensureSwitch(args, "--disable-gpu-sandbox")
	//args = ensureSwitch(args, "--disable-gpu-vsync")
	//args = ensureSwitch(args, "--disable-gpu-shader-disk-cache")
	//args = ensureSwitch(args, "--disable-gpu-watchdog")
	//args = ensureSwitch(args, "--disable-sync")
	args = ensureSwitch(args, "--in-process-gpu")
	args = ensureSwitch(args, "--metrics-recording-only")
	//args = ensureSwitch(args, "--disable-features=UseSkiaRenderer,Vulkan,VaapiVideoDecoder,VaapiVideoEncoder,MediaRouter,AutofillServerCommunication,CertificateTransparencyComponentUpdater,OptimizationGuideModelDownloading,BackgroundFetch,NotificationTriggers")
	return args
}

func ensureSwitch(args []string, value string) []string {
	key := value
	if idx := strings.IndexByte(value, '='); idx >= 0 {
		key = value[:idx]
	}
	for _, existing := range args[1:] {
		if existing == key || existing == value || strings.HasPrefix(existing, key+"=") {
			return args
		}
	}
	return append(args, value)
}

func argvPointer(argv []*C.char) **C.char {
	if len(argv) == 0 {
		return nil
	}
	return (**C.char)(unsafe.Pointer(&argv[0]))
}

func lookupBackend(id uintptr) *linuxBrowserBackend {
	value, ok := browserByID.Load(id)
	if !ok {
		return nil
	}
	backend, _ := value.(*linuxBrowserBackend)
	return backend
}

func cefModifiers(modifiers desktop.Modifier) C.uint32_t {
	var value C.uint32_t
	if modifiers&fyne.KeyModifierShift != 0 {
		value |= C.uint32_t(C.EVENTFLAG_SHIFT_DOWN)
	}
	if modifiers&fyne.KeyModifierControl != 0 {
		value |= C.uint32_t(C.EVENTFLAG_CONTROL_DOWN)
	}
	if modifiers&fyne.KeyModifierAlt != 0 {
		value |= C.uint32_t(C.EVENTFLAG_ALT_DOWN)
	}
	if modifiers&fyne.KeyModifierSuper != 0 {
		value |= C.uint32_t(C.EVENTFLAG_COMMAND_DOWN)
	}
	return value
}

func cefMouseButton(button desktop.MouseButton) C.int {
	switch button {
	case desktop.MouseButtonSecondary:
		return C.int(C.MBT_RIGHT)
	case desktop.MouseButtonTertiary:
		return C.int(C.MBT_MIDDLE)
	default:
		return C.int(C.MBT_LEFT)
	}
}

func cefMouseButtonsModifier(buttons desktop.MouseButton) C.uint32_t {
	var value C.uint32_t
	if buttons&desktop.MouseButtonPrimary != 0 {
		value |= C.uint32_t(C.EVENTFLAG_LEFT_MOUSE_BUTTON)
	}
	if buttons&desktop.MouseButtonSecondary != 0 {
		value |= C.uint32_t(C.EVENTFLAG_RIGHT_MOUSE_BUTTON)
	}
	if buttons&desktop.MouseButtonTertiary != 0 {
		value |= C.uint32_t(C.EVENTFLAG_MIDDLE_MOUSE_BUTTON)
	}
	return value
}

func cefKeyCode(name string) (int, bool) {
	switch name {
	case "Return", "Enter":
		return 0x0D, true
	case "BackSpace":
		return 0x08, true
	case "Tab":
		return 0x09, true
	case "Escape":
		return 0x1B, true
	case "Space":
		return 0x20, true
	case "Left":
		return 0x25, true
	case "Up":
		return 0x26, true
	case "Right":
		return 0x27, true
	case "Down":
		return 0x28, true
	case "Delete":
		return 0x2E, true
	case "Home":
		return 0x24, true
	case "End":
		return 0x23, true
	case "PageUp":
		return 0x21, true
	case "PageDown":
		return 0x22, true
	case "LeftShift", "RightShift":
		return 0x10, true
	case "LeftControl", "RightControl":
		return 0x11, true
	case "LeftAlt", "RightAlt":
		return 0x12, true
	}
	if len(name) == 1 {
		return int(strings.ToUpper(name)[0]), true
	}
	if strings.HasPrefix(name, "F") {
		if n, err := strconv.Atoi(strings.TrimPrefix(name, "F")); err == nil && n >= 1 && n <= 24 {
			return 0x70 + (n - 1), true
		}
	}
	return 0, false
}

func dispatchToFyne(fn func()) {
	if fyne.CurrentApp() == nil {
		fn()
		return
	}
	fyne.Do(fn)
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
	case 1:
		return "sub_frame"
	case 2:
		return "stylesheet"
	case 3:
		return "script"
	case 4:
		return "image"
	case 5:
		return "font_resource"
	case 6:
		return "sub_resource"
	case 7:
		return "object"
	case 8:
		return "media"
	case 9:
		return "worker"
	case 10:
		return "shared_worker"
	case 11:
		return "prefetch"
	case 12:
		return "favicon"
	case 13:
		return "xhr"
	case 14:
		return "ping"
	case 15:
		return "service_worker"
	case 16:
		return "csp_report"
	case 17:
		return "plugin_resource"
	case 19:
		return "navigation_preload_main_frame"
	case 20:
		return "navigation_preload_sub_frame"
	default:
		return fmt.Sprintf("resource_%d", value)
	}
}

func mapCEFCursor(cursorType int) desktop.StandardCursor {
	switch cursorType {
	case cefCursorCross, cefCursorCell:
		return desktop.CrosshairCursor
	case cefCursorHand:
		return desktop.PointerCursor
	case cefCursorIBeam, cefCursorVerticalText:
		return desktop.TextCursor
	case cefCursorEastResize, cefCursorWestResize, cefCursorEastWestResize, cefCursorColumnResize:
		return desktop.HResizeCursor
	case cefCursorNorthResize, cefCursorSouthResize, cefCursorNorthSouthResize, cefCursorRowResize:
		return desktop.VResizeCursor
	case cefCursorNone:
		return desktop.HiddenCursor
	case cefCursorPointer, cefCursorCustom:
		return desktop.DefaultCursor
	default:
		return desktop.DefaultCursor
	}
}

func (b *linuxBrowserBackend) replaceContextMenu(session *contextMenuSession) *contextMenuSession {
	b.mu.Lock()
	defer b.mu.Unlock()
	previous := b.contextMenu
	b.contextMenu = session
	return previous
}

func (b *linuxBrowserBackend) clearContextMenu(session *contextMenuSession) {
	b.mu.Lock()
	if b.contextMenu == session {
		b.contextMenu = nil
	}
	b.mu.Unlock()
}

func (b *linuxBrowserBackend) finishContextMenu(session *contextMenuSession, selected bool, commandID int) {
	if session == nil || !session.done.CompareAndSwap(false, true) {
		return
	}
	b.clearContextMenu(session)
	if selected {
		_ = b.runtime.thread.run(func() error {
			C.fynecef_context_menu_continue(session.native, C.int(commandID), 0)
			return nil
		})
		return
	}
	_ = b.runtime.thread.run(func() error {
		C.fynecef_context_menu_cancel(session.native)
		return nil
	})
}

func (b *linuxBrowserBackend) cancelAndHideContextMenu(session *contextMenuSession) {
	if session == nil {
		return
	}
	if session.popup != nil {
		session.popup.Hide()
	}
	b.finishContextMenu(session, false, 0)
}

func normalizeContextMenuLabel(label string) string {
	label = strings.ReplaceAll(label, "&", "")
	label = strings.ReplaceAll(label, "…", "...")
	label = strings.ToLower(strings.TrimSpace(label))
	return strings.Join(strings.Fields(label), " ")
}

func isUnsupportedContextMenuItem(commandID int, label string) bool {
	switch commandID {
	case cefMenuIDFind, cefMenuIDPrint, cefMenuIDViewSource:
		return true
	}

	normalized := normalizeContextMenuLabel(label)
	if normalized == "" {
		return false
	}

	unsupportedPhrases := []string{
		"open link in new tab",
		"open link in new window",
		"open link in incognito",
		"open image in new tab",
		"open image in new window",
		"open video in new tab",
		"open video in new window",
		"open audio in new tab",
		"open audio in new window",
		"open frame in new tab",
		"open frame in new window",
		"save link as",
		"save image as",
		"save video as",
		"save audio as",
		"save page as",
		"view page source",
		"view frame source",
		"inspect",
		"inspect element",
		"developer tools",
		"devtools",
	}
	for _, phrase := range unsupportedPhrases {
		if strings.Contains(normalized, phrase) {
			return true
		}
	}

	return false
}

func buildContextMenuItems(b *linuxBrowserBackend, session *contextMenuSession, items *C.fynecef_menu_item_t, count int) []*fyne.MenuItem {
	if items == nil || count <= 0 {
		return nil
	}

	source := unsafe.Slice(items, count)
	menuItems := make([]*fyne.MenuItem, 0, count)
	for _, item := range source {
		itemType := int(item._type)
		switch itemType {
		case cefMenuItemTypeSeparator:
			menuItems = append(menuItems, fyne.NewMenuItemSeparator())
		case cefMenuItemTypeCommand, cefMenuItemTypeCheck, cefMenuItemTypeRadio, cefMenuItemTypeSubmenu:
			label := ""
			if item.label != nil {
				// Fyne does not support mnemonics, so remove the '&' used by CEF to indicate them
				label = strings.ReplaceAll(C.GoString(item.label), "&", "")
			}
			commandID := int(item.command_id)
			if isUnsupportedContextMenuItem(commandID, label) {
				continue
			}
			menuItem := fyne.NewMenuItem(label, nil)
			menuItem.Disabled = item.enabled == 0
			menuItem.Checked = item.checked != 0
			if item.child_count > 0 && item.children != nil {
				children := buildContextMenuItems(b, session, item.children, int(item.child_count))
				if len(children) == 0 {
					continue
				}
				menuItem.ChildMenu = fyne.NewMenu("", children...)
			} else if itemType != cefMenuItemTypeSubmenu {
				menuItem.Action = func(commandID int) func() {
					return func() {
						b.finishContextMenu(session, true, commandID)
					}
				}(commandID)
			}
			menuItems = append(menuItems, menuItem)
		}
	}

	filtered := make([]*fyne.MenuItem, 0, len(menuItems))
	lastWasSeparator := true
	for _, item := range menuItems {
		if item.IsSeparator {
			if lastWasSeparator {
				continue
			}
			lastWasSeparator = true
			filtered = append(filtered, item)
			continue
		}
		lastWasSeparator = false
		filtered = append(filtered, item)
	}
	if n := len(filtered); n > 0 && filtered[n-1].IsSeparator {
		filtered = filtered[:n-1]
	}
	return filtered
}

func (b *linuxBrowserBackend) showContextMenu(menu *C.fynecef_context_menu_t) {
	if menu == nil {
		return
	}

	app := fyne.CurrentApp()
	if app == nil || app.Driver() == nil {
		_ = b.runtime.thread.run(func() error {
			C.fynecef_context_menu_cancel(menu)
			return nil
		})
		return
	}

	canvas := app.Driver().CanvasForObject(b.browser)
	if canvas == nil {
		_ = b.runtime.thread.run(func() error {
			C.fynecef_context_menu_cancel(menu)
			return nil
		})
		return
	}

	session := &contextMenuSession{native: menu}
	items := buildContextMenuItems(b, session, menu.items, int(menu.item_count))
	if len(items) == 0 {
		b.finishContextMenu(session, false, 0)
		return
	}

	popup := widget.NewPopUpMenu(fyne.NewMenu("", items...), canvas)
	session.popup = popup

	previous := b.replaceContextMenu(session)
	if previous != nil {
		b.cancelAndHideContextMenu(previous)
	}

	hide := popup.OnDismiss
	popup.OnDismiss = func() {
		if hide != nil {
			hide()
		}
		time.AfterFunc(10*time.Millisecond, func() {
			b.finishContextMenu(session, false, 0)
		})
	}

	base := app.Driver().AbsolutePositionForObject(b.browser)
	popup.ShowAtPosition(base.Add(fyne.NewPos(float32(menu.x), float32(menu.y))))
}

//export goCEFOnAddressChange
func goCEFOnAddressChange(handle C.uintptr_t, url *C.char) {
	backend := lookupBackend(uintptr(handle))
	if backend == nil {
		return
	}
	value := C.GoString(url)
	dispatchToFyne(func() {
		backend.browser.setAddress(value)
	})
}

//export goCEFOnTitleChange
func goCEFOnTitleChange(handle C.uintptr_t, title *C.char) {
	backend := lookupBackend(uintptr(handle))
	if backend == nil {
		return
	}
	value := C.GoString(title)
	dispatchToFyne(func() {
		backend.browser.setTitle(value)
	})
}

//export goCEFOnLoadProgress
func goCEFOnLoadProgress(handle C.uintptr_t, progress C.double) {
	backend := lookupBackend(uintptr(handle))
	if backend == nil {
		return
	}
	dispatchToFyne(func() {
		state := snapshot(backend.browser)
		backend.browser.emitProgress(LoadProgress{
			URL:          state.url,
			Title:        state.title,
			Progress:     float64(progress),
			IsLoading:    state.isLoading,
			CanGoBack:    state.canGoBack,
			CanGoForward: state.canGoForward,
		})
	})
}

//export goCEFOnCursorChange
func goCEFOnCursorChange(handle C.uintptr_t, cursorType C.int) {
	backend := lookupBackend(uintptr(handle))
	if backend == nil {
		return
	}
	cursor := mapCEFCursor(int(cursorType))
	dispatchToFyne(func() {
		backend.browser.setCursor(cursor)
	})
}

//export goCEFOnContextMenu
func goCEFOnContextMenu(handle C.uintptr_t, menu *C.fynecef_context_menu_t) {
	backend := lookupBackend(uintptr(handle))
	if backend == nil || menu == nil {
		if menu != nil {
			C.fynecef_context_menu_cancel(menu)
		}
		return
	}
	dispatchToFyne(func() {
		backend.showContextMenu(menu)
	})
}

//export goCEFOnLoadingStateChange
func goCEFOnLoadingStateChange(handle C.uintptr_t, isLoading C.int, canGoBack C.int, canGoForward C.int) {
	backend := lookupBackend(uintptr(handle))
	if backend == nil {
		return
	}
	dispatchToFyne(func() {
		state := snapshot(backend.browser)
		backend.browser.emitProgress(LoadProgress{
			URL:          state.url,
			Title:        state.title,
			Progress:     state.progress,
			IsLoading:    isLoading != 0,
			CanGoBack:    canGoBack != 0,
			CanGoForward: canGoForward != 0,
		})
	})
}

//export goCEFOnLoadError
func goCEFOnLoadError(handle C.uintptr_t, code C.int, errorText *C.char, failedURL *C.char) {
	backend := lookupBackend(uintptr(handle))
	if backend == nil {
		return
	}
	if int(code) == -3 {
		return
	}
	text := C.GoString(errorText)
	url := C.GoString(failedURL)
	dispatchToFyne(func() {
		backend.browser.emitError(fmt.Errorf("fynecef: load error %d: %s (%s)", int(code), text, url))
	})
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

//export goCEFOnFrame
func goCEFOnFrame(handle C.uintptr_t, buffer unsafe.Pointer, width C.int, height C.int, stride C.int, dirtyRectCount C.size_t, dirtyRects *C.cef_rect_t) {
	backend := lookupBackend(uintptr(handle))
	if backend == nil || buffer == nil || width <= 0 || height <= 0 || stride <= 0 {
		return
	}

	frameWidth := int(width)
	frameHeight := int(height)
	frameStride := int(stride)

	copyFrameRect := func(x, y, rectWidth, rectHeight int) []byte {
		copyStride := rectWidth * 4
		size := rectHeight * copyStride
		if size <= 0 || size%4 != 0 {
			return nil
		}
		raw := make([]byte, size)
		C.fynecef_copy_bgra_rect_to_rgba(
			(*C.uint8_t)(unsafe.Pointer(&raw[0])),
			C.int(copyStride),
			(*C.uint8_t)(buffer),
			stride,
			C.int(x),
			C.int(y),
			C.int(rectWidth),
			C.int(rectHeight),
		)
		return raw
	}

	fullFrame := func() {
		raw := copyFrameRect(0, 0, frameWidth, frameHeight)
		if raw == nil {
			return
		}
		backend.browser.queueFrame(frameWidth, frameHeight, frameStride, raw)
	}

	if dirtyRectCount == 0 || dirtyRects == nil {
		fullFrame()
		return
	}

	rects := unsafe.Slice(dirtyRects, int(dirtyRectCount))
	clippedRects := make([]frameRect, 0, len(rects))
	for _, rect := range rects {
		rectX := int(rect.x)
		rectY := int(rect.y)
		rectWidth := int(rect.width)
		rectHeight := int(rect.height)

		if rectX < 0 {
			rectWidth += rectX
			rectX = 0
		}
		if rectY < 0 {
			rectHeight += rectY
			rectY = 0
		}
		if rectX >= frameWidth || rectY >= frameHeight {
			continue
		}
		if rectX+rectWidth > frameWidth {
			rectWidth = frameWidth - rectX
		}
		if rectY+rectHeight > frameHeight {
			rectHeight = frameHeight - rectY
		}
		if rectWidth <= 0 || rectHeight <= 0 {
			continue
		}

		if rectX == 0 && rectY == 0 && rectWidth == frameWidth && rectHeight == frameHeight {
			fullFrame()
			return
		}

		clippedRects = append(clippedRects, frameRect{
			x:      rectX,
			y:      rectY,
			width:  rectWidth,
			height: rectHeight,
		})
	}

	if len(clippedRects) == 0 {
		fullFrame()
		return
	}
	if !backend.browser.queueFrameRects(frameWidth, frameHeight, frameStride, clippedRects, func(dst []byte, dstStride int) {
		for _, rect := range clippedRects {
			dstStart := rect.y*dstStride + rect.x*4
			C.fynecef_copy_bgra_rect_to_rgba(
				(*C.uint8_t)(unsafe.Pointer(&dst[dstStart])),
				C.int(dstStride),
				(*C.uint8_t)(buffer),
				stride,
				C.int(rect.x),
				C.int(rect.y),
				C.int(rect.width),
				C.int(rect.height),
			)
		}
	}) {
		fullFrame()
	}
}
