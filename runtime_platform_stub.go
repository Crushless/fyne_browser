//go:build cgo && windows

package fynecef

func MaybeRunSubprocess() (bool, int) {
	return false, 0
}

func Init(RuntimeOptions) error {
	return ErrPlatformUnsupported
}

func Shutdown() {
}
