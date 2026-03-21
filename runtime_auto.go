//go:build cgo && (linux || darwin || windows)

package fynecef

import "os"

func init() {
	if handled, code := MaybeRunSubprocess(); handled {
		os.Exit(code)
	}
}
