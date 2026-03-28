[![GitHub go.mod Go version of a Go module](https://img.shields.io/github/go-mod/go-version/crushless/fyne_browser.svg)](https://github.com/crushless/fyne_browser)
[![Go](https://github.com/Crushless/fyne_browser/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/Crushless/fyne_browser/actions/workflows/go.yml)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](https://opensource.org/licenses/BSD-3-Clause)  
![Linux](https://img.shields.io/badge/Linux-FCC624?style=flat&logo=linux&logoColor=black)

# fynecef

`fynecef` is a Fyne v2 browser addon that embeds the Chromium Embedded Framework (CEF) inside a Fyne widget.

It is designed to feel like a normal Fyne component:

- layoutable inside any Fyne container
- interactive, including links, mouse input, keyboard input, and navigation
- able to load remote websites
- able to auto-discover or auto-download a matching CEF runtime
- able to report page loading progress back to Go
- able to intercept resource requests from Go for filtering or blocking

## Status

The project is functional, but not yet feature-complete on every desktop platform.

Current status:

- Linux with `cgo` enabled: supported
- Linux session type: X11 required for the current backend (but works with Xwayland)
- macOS: supported with the same CEF-based backend family used on Linux
- macOS request interception: supported through CEF request callbacks
- Windows: framework discovery/download is wired up, native embedding is not implemented yet

## Features

- `fynecef.Browser` widget for embedding web content into a Fyne window
- automatic CEF runtime discovery
- automatic download of the latest matching official CEF binary when needed
- load progress callbacks
- address and title change callbacks
- resource interception callback for ad blocking, request filtering, or policy enforcement
- demo browser application with address bar, navigation buttons, stop/reload, and progress bar
- optional bootstrap CLI for prefetching CEF in development or CI

## Requirements

- Go `1.26+`
- `cgo` enabled
- for Linux runtime use today: X11 (Xwayland compatible)
- internet access on first run if CEF must be downloaded automatically

Notes:

- No custom build tag is required on supported platforms.
- Importing the module no longer requires a repo-local CEF SDK at build time; the C headers are vendored and `libcef` is loaded from the discovered runtime at startup.
- Subprocess handling is automatic once the package is imported.
- If `cgo` is disabled, the package still builds, but the browser backend falls back to a clear unsupported message.

## Quick Start

### Run the Demo

```bash
go run ./cmd/demo
```

The demo will:

1. initialize the CEF runtime
2. try to find an existing CEF installation
3. download a matching CEF build automatically if needed
4. open a simple Fyne browser shell

### Optional: Prefetch the CEF Runtime

If you want to download CEF ahead of time instead of waiting for first run:

```bash
go run ./cmd/cefbootstrap
```

This installs the latest matching framework under `third_party/cef/builds` and updates `third_party/cef/current`.
That bootstrap step is optional for consumers of the published Go module and mainly useful for local development, CI prefetching, or deterministic packaging.

To prefetch a specific target such as Intel macOS, pass `-platform` explicitly:

```bash
go run ./cmd/cefbootstrap -platform macosx64
```

## Minimal Usage

The simplest integration is to create a browser widget and let `fynecef` initialize itself automatically.

Note:
The import path below uses the module name from this workspace. Replace it with your published module path when you publish this repository.

```go
package main

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"

	fynecef "fyne_browser"
)

func main() {
	a := app.New()
	w := a.NewWindow("fynecef example")
	w.Resize(fyne.NewSize(1200, 800))

	browser, err := fynecef.NewBrowser(fynecef.BrowserOptions{
		Window:     w,
		InitialURL: "https://example.com",
	})
	if err != nil {
		log.Println("browser init:", err)
	}

	w.SetContent(container.NewMax(browser))
	w.ShowAndRun()
}
```

## Recommended Initialization

If you want explicit control over where CEF is found, downloaded, and cached, call `fynecef.Init(...)` before creating your Fyne app:

```go
if err := fynecef.Init(fynecef.RuntimeOptions{
	FrameworkRoot: "third_party/cef/current",
	SearchPaths: []string{
		"third_party/cef/current",
		"third_party/cef/builds",
	},
	DownloadDir:   "third_party/cef/builds",
	AllowDownload: true,
	ManifestURL:   fynecef.OfficialManifestURL,
	Channel:       fynecef.ChannelStable,
	PackageType:   fynecef.PackageTypeMinimal,
	CachePath:     "/tmp/fynecef-profile",
}); err != nil {
	return err
}
```

This is useful when you want:

- deterministic download locations
- preconfigured cache/profile locations
- early failure before the UI is created
- tighter control over packaging and deployment

## Callbacks Example

`fynecef` exposes browser callbacks through `BrowserOptions.Callbacks`.

```go
browser, err := fynecef.NewBrowser(fynecef.BrowserOptions{
	Window:     w,
	InitialURL: "https://example.com",
	Callbacks: fynecef.Callbacks{
		OnBeforeResourceLoad: func(req fynecef.ResourceRequest) fynecef.ResourceDecision {
			if strings.Contains(req.URL, "doubleclick.net") {
				return fynecef.ResourceBlock
			}
			return fynecef.ResourceAllow
		},
		OnLoadProgress: func(p fynecef.LoadProgress) {
			log.Printf("loading=%v progress=%.0f%% url=%s", p.IsLoading, p.Progress*100, p.URL)
		},
		OnAddressChange: func(url string) {
			log.Println("address:", url)
		},
		OnTitleChange: func(title string) {
			log.Println("title:", title)
		},
		OnError: func(err error) {
			log.Println("browser error:", err)
		},
	},
})
```

The callback types are defined in [types.go](types.go).

## How Runtime Discovery Works

When the runtime starts, `fynecef`:

1. checks any explicitly configured `FrameworkRoot`
2. checks configured `SearchPaths`
3. checks default local cache locations
4. downloads a matching official CEF build if allowed

The runtime discovery and initialization path is implemented in [runtime_linux_cef.go](runtime_linux_cef.go) and [download.go](download.go).

By default, automatic downloads use the official Spotify-hosted CEF build index:

- manifest URL: `https://cef-builds.spotifycdn.com/index.json`
- default channel: `stable`
- default package type: `minimal`

## Demo Application

The demo application lives at [cmd/demo/main.go](cmd/demo/main.go).

It includes:

- address bar
- back/forward navigation
- reload and stop buttons
- load progress bar
- title/status updates

Run it with:

```bash
go run ./cmd/demo
```

## Platform Support

| Platform | Status | Notes |
| --- | --- | --- |
| Linux | Supported | Current implementation targets X11 and uses CEF windowless rendering embedded into the Fyne widget |
| macOS | Supported | Uses CEF windowless rendering with the same callback model as Linux |
| Windows | Not yet implemented | Framework discovery/download is present, native embedding is still pending |

## Repository Layout

- [browser.go](browser.go): Fyne widget implementation
- [runtime_linux_cef.go](runtime_linux_cef.go): Linux runtime and native bridge
- [cef_linux.c](cef_linux.c): CEF C bridge for Linux
- [download.go](download.go): framework discovery, download, and extraction logic
- [cmd/demo/main.go](cmd/demo/main.go): demo browser application
- [cmd/cefbootstrap/main.go](cmd/cefbootstrap/main.go): optional bootstrap CLI

## Development Roadmap

| Feature | Likeliness | Comment | Issue |
|:--|:-:|:--|:-:|
| Windows support | 🟢🟢 | planned before v1.0 |  |
| macOS support | 🟢🟢 | planned  before v1.0 |  |
| Performance improvements | 🟡 | maybe possible by shared pixmap between `fyne` and `CEF`  | - |
| Mobile support | 🟠 | maybe by using the platform native html renderer | - |
| Web support | 🔴 | unlikely, because of platform restrictions | - |

## Licensing

### This Repository

The `fynecef` source code in this repository is licensed under the BSD 3-Clause License. See [LICENSE](LICENSE).

### CEF Runtime

The CEF framework itself is not covered by this repository's `LICENSE` file.

Upstream licensing references:

- [CEF upstream LICENSE.txt](https://github.com/chromiumembedded/cef/blob/master/LICENSE.txt): CEF license text
- [CEF binary downloads](https://cef-builds.spotifycdn.com/index.html): official binary distributions that include redistribution notes and bundled notices

If you bootstrap or auto-download CEF in a local checkout, the downloaded runtime also includes these local notice files:

- `third_party/cef/current/LICENSE.txt`
- `third_party/cef/current/CREDITS.html`
- `third_party/cef/current/README.txt`

Those files are generated under `third_party/`, which is gitignored, so this README does not link to them directly.

According to the CEF binary distribution included in this workspace, the CEF project is BSD licensed, and additional Chromium/third-party software in the distribution is covered by other licenses. If you redistribute an application bundled with CEF, you should make sure the required license texts and notices are included with your distribution.

## License Summary

- `fynecef` source code: BSD 3-Clause License
- downloaded CEF framework: BSD-licensed CEF plus additional Chromium/third-party license notices
- Fyne itself: BSD 3-Clause License
