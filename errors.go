package fynecef

import "errors"

var (
	ErrCEFNotBuilt         = errors.New("fynecef: native CEF support is not built in for this binary")
	ErrNoFramework         = errors.New("fynecef: no usable CEF framework was found")
	ErrNoBuild             = errors.New("fynecef: no matching official CEF build was found")
	ErrBlockedByHook       = errors.New("fynecef: resource load blocked by Go callback")
	ErrWindowRequired      = errors.New("fynecef: BrowserOptions.Window is required for native embedding")
	ErrRuntimeInit         = errors.New("fynecef: runtime initialization failed")
	ErrPlatformUnsupported = errors.New("fynecef: native CEF embedding is not implemented on this platform yet")
)
