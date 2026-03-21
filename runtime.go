//go:build !cgo || (!linux && !darwin && !windows)

package fynecef

func MaybeRunSubprocess() (bool, int) {
	return false, 0
}

func Init(RuntimeOptions) error {
	return ErrCEFNotBuilt
}

func Shutdown() {
}
