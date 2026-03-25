package fynecef

import (
	"image"
	"image/color"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

var (
	_ fyne.Widget        = (*Browser)(nil)
	_ fyne.Draggable     = (*Browser)(nil)
	_ fyne.Focusable     = (*Browser)(nil)
	_ fyne.Scrollable    = (*Browser)(nil)
	_ desktop.Mouseable  = (*Browser)(nil)
	_ desktop.Hoverable  = (*Browser)(nil)
	_ desktop.Keyable    = (*Browser)(nil)
	_ desktop.Cursorable = (*Browser)(nil)
)

type Browser struct {
	widget.BaseWidget

	options BrowserOptions
	backend browserBackend

	mu           sync.RWMutex
	frame        *image.NRGBA
	imageObject  *canvas.Image
	statusObject *canvas.Text
	unavailable  string
	focused      bool
	currentURL   string
	currentTitle string
	canGoBack    bool
	canGoForward bool
	loading      bool
	loadingPct   float64

	pendingFrame        *framePayload
	frameDispatchQueued bool
	boundsSyncScheduled bool
	boundsSyncDirty     bool
	lastBoundsSync      time.Time
	activeFrameRate     int
	frameRateRestoreSeq uint64
	cursor              desktop.StandardCursor
	activeMouseButtons  desktop.MouseButton
	activeModifiers     desktop.Modifier
	lastClickAt         time.Time
	lastClickPos        fyne.Position
	lastClickButton     desktop.MouseButton
	lastClickCount      int
	pressedClickCount   int
}

type framePayload struct {
	width  int
	height int
	stride int
	pixels []byte
}

const boundsSyncInterval = 16 * time.Millisecond
const defaultWindowlessFrameRate = 60
const resizeWindowlessFrameRate = 20
const resizeFrameRateRestoreDelay = 180 * time.Millisecond

func NewBrowser(opts BrowserOptions) (*Browser, error) {
	b := &Browser{
		options:         opts,
		frame:           image.NewNRGBA(image.Rect(0, 0, 1, 1)),
		activeFrameRate: defaultWindowlessFrameRate,
		cursor:          desktop.DefaultCursor,
	}
	b.imageObject = canvas.NewImageFromImage(b.frame)
	b.imageObject.FillMode = canvas.ImageFillStretch
	b.statusObject = canvas.NewText("", color.NRGBA{R: 235, G: 238, B: 242, A: 255})
	b.statusObject.Alignment = fyne.TextAlignCenter
	b.ExtendBaseWidget(b)

	backend, err := newBrowserBackend(b, opts)
	b.backend = backend
	if err != nil {
		b.setUnavailable(err.Error())
		return b, err
	}
	if opts.InitialURL != "" {
		if loadErr := b.LoadURL(opts.InitialURL); loadErr != nil && err == nil {
			err = loadErr
		}
	}
	return b, err
}

func (b *Browser) CreateRenderer() fyne.WidgetRenderer {
	background := canvas.NewRectangle(color.NRGBA{R: 24, G: 28, B: 35, A: 255})
	return &browserRenderer{
		browser:    b,
		background: background,
		objects:    []fyne.CanvasObject{background, b.imageObject, b.statusObject},
	}
}

func (b *Browser) MinSize() fyne.Size {
	return fyne.NewSize(240, 160)
}

func (b *Browser) LoadURL(rawURL string) error {
	if b.backend == nil {
		return ErrCEFNotBuilt
	}
	return b.backend.LoadURL(rawURL)
}

func (b *Browser) Reload() error {
	if b.backend == nil {
		return ErrCEFNotBuilt
	}
	return b.backend.Reload()
}

func (b *Browser) Stop() error {
	if b.backend == nil {
		return ErrCEFNotBuilt
	}
	return b.backend.Stop()
}

func (b *Browser) GoBack() error {
	if b.backend == nil {
		return ErrCEFNotBuilt
	}
	return b.backend.GoBack()
}

func (b *Browser) GoForward() error {
	if b.backend == nil {
		return ErrCEFNotBuilt
	}
	return b.backend.GoForward()
}

func (b *Browser) Close() error {
	if b.backend == nil {
		return nil
	}
	return b.backend.Close()
}

func (b *Browser) CurrentURL() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.currentURL
}

func (b *Browser) Title() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.currentTitle
}

func (b *Browser) CanGoBack() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.canGoBack
}

func (b *Browser) CanGoForward() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.canGoForward
}

func (b *Browser) LoadingProgress() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.loadingPct
}

func (b *Browser) Cursor() desktop.Cursor {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.cursor
}

func (b *Browser) FocusGained() {
	b.mu.Lock()
	b.focused = true
	b.mu.Unlock()
	if b.backend != nil {
		_ = b.backend.Focus(true)
	}
}

func (b *Browser) FocusLost() {
	b.mu.Lock()
	b.focused = false
	b.mu.Unlock()
	if b.backend != nil {
		_ = b.backend.Focus(false)
	}
}

func (b *Browser) Focused() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.focused
}

func (b *Browser) TypedRune(r rune) {
	if b.backend != nil {
		_ = b.backend.Rune(r)
	}
}

func (b *Browser) TypedKey(ev *fyne.KeyEvent) {
	if ev == nil || b.backend == nil {
		return
	}
	_ = b.backend.KeyDown(string(ev.Name), 0)
	_ = b.backend.KeyUp(string(ev.Name), 0)
}

func (b *Browser) KeyDown(ev *fyne.KeyEvent) {
	if ev == nil || b.backend == nil {
		return
	}
	_ = b.backend.KeyDown(string(ev.Name), 0)
}

func (b *Browser) KeyUp(ev *fyne.KeyEvent) {
	if ev == nil || b.backend == nil {
		return
	}
	_ = b.backend.KeyUp(string(ev.Name), 0)
}

func (b *Browser) MouseDown(ev *desktop.MouseEvent) {
	b.focusSelf()
	if ev == nil || b.backend == nil {
		return
	}
	clickCount := b.nextClickCount(ev.Position, ev.Button)
	b.mu.Lock()
	b.activeMouseButtons |= ev.Button
	b.activeModifiers = ev.Modifier
	b.pressedClickCount = clickCount
	b.mu.Unlock()
	_ = b.backend.MouseDown(int(ev.Position.X), int(ev.Position.Y), ev.Button, ev.Modifier, clickCount)
}

func (b *Browser) MouseUp(ev *desktop.MouseEvent) {
	if ev == nil || b.backend == nil {
		return
	}
	b.mu.Lock()
	clickCount := b.pressedClickCount
	if clickCount == 0 {
		clickCount = 1
	}
	b.activeMouseButtons &^= ev.Button
	b.activeModifiers = ev.Modifier
	b.pressedClickCount = 0
	b.mu.Unlock()
	_ = b.backend.MouseUp(int(ev.Position.X), int(ev.Position.Y), ev.Button, ev.Modifier, clickCount)
}

func (b *Browser) MouseIn(ev *desktop.MouseEvent) {
	if ev == nil || b.backend == nil {
		return
	}
	if ev.Button != 0 {
		_ = b.backend.DragMove(int(ev.Position.X), int(ev.Position.Y), ev.Button, ev.Modifier)
		return
	}
	_ = b.backend.MouseMove(int(ev.Position.X), int(ev.Position.Y), ev.Modifier)
}

func (b *Browser) MouseMoved(ev *desktop.MouseEvent) {
	if ev == nil || b.backend == nil {
		return
	}
	b.mu.Lock()
	if ev.Button != 0 {
		b.activeMouseButtons = ev.Button
	}
	b.activeModifiers = ev.Modifier
	b.mu.Unlock()
	if ev.Button != 0 {
		_ = b.backend.DragMove(int(ev.Position.X), int(ev.Position.Y), ev.Button, ev.Modifier)
		return
	}
	_ = b.backend.MouseMove(int(ev.Position.X), int(ev.Position.Y), ev.Modifier)
}

func (b *Browser) MouseOut() {
}

func (b *Browser) Dragged(ev *fyne.DragEvent) {
	if ev == nil || b.backend == nil {
		return
	}
	b.mu.RLock()
	buttons := b.activeMouseButtons
	modifiers := b.activeModifiers
	b.mu.RUnlock()
	if buttons == 0 {
		_ = b.backend.MouseMove(int(ev.Position.X), int(ev.Position.Y), modifiers)
		return
	}
	_ = b.backend.DragMove(int(ev.Position.X), int(ev.Position.Y), buttons, modifiers)
}

func (b *Browser) DragEnd() {
}

func (b *Browser) Scrolled(ev *fyne.ScrollEvent) {
	if ev == nil || b.backend == nil {
		return
	}
	_ = b.backend.MouseWheel(int(ev.Position.X), int(ev.Position.Y), int(ev.Scrolled.DX), int(ev.Scrolled.DY), 0)
}

func (b *Browser) Resize(size fyne.Size) {
	b.BaseWidget.Resize(size)
}

func (b *Browser) setUnavailable(message string) {
	b.mu.Lock()
	b.unavailable = message
	b.statusObject.Text = message
	b.mu.Unlock()
	canvas.Refresh(b)
}

func (b *Browser) setFrame(width, height, stride int, pixels []byte) {
	if width <= 0 || height <= 0 || stride < width*4 || len(pixels) < height*stride {
		return
	}
	img := &image.NRGBA{
		Pix:    pixels,
		Stride: stride,
		Rect:   image.Rect(0, 0, width, height),
	}

	b.mu.Lock()
	b.frame = img
	b.imageObject.Image = img
	b.unavailable = ""
	b.statusObject.Text = ""
	b.mu.Unlock()

	canvas.Refresh(b.imageObject)
}

func (b *Browser) queueFrame(width, height, stride int, pixels []byte) {
	if width <= 0 || height <= 0 || stride < width*4 || len(pixels) < height*stride {
		return
	}

	schedule := false

	b.mu.Lock()
	b.pendingFrame = &framePayload{
		width:  width,
		height: height,
		stride: stride,
		pixels: pixels,
	}
	if !b.frameDispatchQueued {
		b.frameDispatchQueued = true
		schedule = true
	}
	b.mu.Unlock()

	if !schedule {
		return
	}

	dispatch := b.applyQueuedFrame
	if fyne.CurrentApp() == nil {
		dispatch()
		return
	}
	fyne.Do(dispatch)
}

func (b *Browser) applyQueuedFrame() {
	b.mu.Lock()
	frame := b.pendingFrame
	b.pendingFrame = nil
	b.frameDispatchQueued = false
	b.mu.Unlock()

	if frame == nil {
		return
	}
	b.setFrame(frame.width, frame.height, frame.stride, frame.pixels)
}

func (b *Browser) setAddress(rawURL string) {
	b.mu.Lock()
	b.currentURL = rawURL
	b.mu.Unlock()
	if fn := b.options.Callbacks.OnAddressChange; fn != nil {
		fn(rawURL)
	}
	canvas.Refresh(b)
}

func (b *Browser) setCursor(cursor desktop.StandardCursor) {
	b.mu.Lock()
	if b.cursor == cursor {
		b.mu.Unlock()
		return
	}
	b.cursor = cursor
	b.mu.Unlock()
	canvas.Refresh(b)
}

func (b *Browser) nextClickCount(pos fyne.Position, button desktop.MouseButton) int {
	count := 1
	now := time.Now()

	delay := 300 * time.Millisecond
	if app := fyne.CurrentApp(); app != nil && app.Driver() != nil {
		delay = app.Driver().DoubleTapDelay()
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if button == b.lastClickButton &&
		now.Sub(b.lastClickAt) <= delay &&
		abs32(pos.X-b.lastClickPos.X) <= 4 &&
		abs32(pos.Y-b.lastClickPos.Y) <= 4 {
		count = b.lastClickCount + 1
		if count > 3 {
			count = 1
		}
	}

	b.lastClickAt = now
	b.lastClickPos = pos
	b.lastClickButton = button
	b.lastClickCount = count
	return count
}

func abs32(value float32) float32 {
	if value < 0 {
		return -value
	}
	return value
}

func (b *Browser) setTitle(title string) {
	b.mu.Lock()
	b.currentTitle = title
	b.mu.Unlock()
	if fn := b.options.Callbacks.OnTitleChange; fn != nil {
		fn(title)
	}
	canvas.Refresh(b)
}

func (b *Browser) emitProgress(progress LoadProgress) {
	b.mu.Lock()
	b.loading = progress.IsLoading
	b.loadingPct = progress.Progress
	b.canGoBack = progress.CanGoBack
	b.canGoForward = progress.CanGoForward
	if progress.URL != "" {
		b.currentURL = progress.URL
	}
	if progress.Title != "" {
		b.currentTitle = progress.Title
	}
	b.mu.Unlock()
	if fn := b.options.Callbacks.OnLoadProgress; fn != nil {
		fn(progress)
	}
	canvas.Refresh(b)
}

func (b *Browser) emitError(err error) {
	if err == nil {
		return
	}
	if fn := b.options.Callbacks.OnError; fn != nil {
		fn(err)
	}
	if b.CurrentURL() == "" {
		b.setUnavailable(err.Error())
	}
}

func (b *Browser) focusSelf() {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	driver := app.Driver()
	if driver == nil {
		return
	}
	canvasForObject := driver.CanvasForObject(b)
	if canvasForObject != nil {
		canvasForObject.Focus(b)
	}
}

func (b *Browser) syncBounds() {
	if b.backend == nil {
		return
	}
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	driver := app.Driver()
	if driver == nil {
		return
	}
	pos := driver.AbsolutePositionForObject(b)
	size := b.Size()
	if size.Width <= 0 || size.Height <= 0 {
		return
	}
	_ = b.backend.SetBounds(int(pos.X), int(pos.Y), int(size.Width), int(size.Height))
}

func (b *Browser) requestBoundsSync() {
	if b.backend == nil {
		return
	}
	b.noteResizeActivity()

	delay := time.Duration(0)
	schedule := false

	b.mu.Lock()
	b.boundsSyncDirty = true
	if !b.boundsSyncScheduled {
		delay = boundsSyncInterval - time.Since(b.lastBoundsSync)
		if delay < 0 {
			delay = 0
		}
		b.boundsSyncScheduled = true
		schedule = true
	}
	b.mu.Unlock()

	if schedule {
		b.scheduleBoundsSync(delay)
	}
}

func (b *Browser) noteResizeActivity() {
	b.setFrameRate(resizeWindowlessFrameRate)

	b.mu.Lock()
	b.frameRateRestoreSeq++
	seq := b.frameRateRestoreSeq
	b.mu.Unlock()

	time.AfterFunc(resizeFrameRateRestoreDelay, func() {
		b.mu.Lock()
		if seq != b.frameRateRestoreSeq {
			b.mu.Unlock()
			return
		}
		b.mu.Unlock()
		b.setFrameRate(defaultWindowlessFrameRate)
	})
}

func (b *Browser) setFrameRate(rate int) {
	if b.backend == nil {
		return
	}

	b.mu.Lock()
	if b.activeFrameRate == rate {
		b.mu.Unlock()
		return
	}
	b.activeFrameRate = rate
	b.mu.Unlock()

	_ = b.backend.SetFrameRate(rate)
}

func (b *Browser) scheduleBoundsSync(delay time.Duration) {
	time.AfterFunc(delay, func() {
		if fyne.CurrentApp() == nil {
			b.flushBoundsSync()
			return
		}
		fyne.Do(b.flushBoundsSync)
	})
}

func (b *Browser) flushBoundsSync() {
	b.mu.Lock()
	b.boundsSyncDirty = false
	b.mu.Unlock()

	b.syncBounds()

	b.mu.Lock()
	b.lastBoundsSync = time.Now()
	dirty := b.boundsSyncDirty
	if !dirty {
		b.boundsSyncScheduled = false
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()

	b.scheduleBoundsSync(boundsSyncInterval)
}

type browserRenderer struct {
	browser    *Browser
	background *canvas.Rectangle
	objects    []fyne.CanvasObject
}

func (r *browserRenderer) BackgroundColor() color.Color {
	return color.Transparent
}

func (r *browserRenderer) Destroy() {
}

func (r *browserRenderer) Layout(size fyne.Size) {
	r.background.Resize(size)
	r.browser.imageObject.Resize(size)

	statusSize := r.browser.statusObject.MinSize()
	x := (size.Width - statusSize.Width) / 2
	y := (size.Height - statusSize.Height) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	r.browser.statusObject.Move(fyne.NewPos(x, y))
	r.browser.statusObject.Resize(statusSize)
	r.browser.requestBoundsSync()
}

func (r *browserRenderer) MinSize() fyne.Size {
	return fyne.NewSize(240, 160)
}

func (r *browserRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *browserRenderer) Refresh() {
	r.browser.mu.RLock()
	showStatus := r.browser.unavailable != ""
	r.browser.statusObject.Text = r.browser.unavailable
	r.browser.mu.RUnlock()

	if showStatus {
		r.browser.statusObject.Show()
	} else {
		r.browser.statusObject.Hide()
	}
	canvas.Refresh(r.background)
	canvas.Refresh(r.browser.imageObject)
	canvas.Refresh(r.browser.statusObject)
}
