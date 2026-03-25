package fynecef

import (
	"fmt"
	"strings"
	"sync/atomic"

	"fyne.io/fyne/v2"
)

var fyneDispatchStopped atomic.Bool

func tryFyneDo(fn func()) bool {
	if fn == nil {
		return false
	}
	if fyne.CurrentApp() == nil {
		fn()
		return true
	}
	if fyneDispatchStopped.Load() {
		return false
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			if isClosedFyneQueuePanic(recovered) {
				return
			}
			panic(recovered)
		}
	}()

	fyne.Do(fn)
	return true
}

func isClosedFyneQueuePanic(recovered any) bool {
	return strings.Contains(fmt.Sprint(recovered), "send on closed channel")
}
