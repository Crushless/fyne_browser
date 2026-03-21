package fynecef

import (
	"context"
	"net/http"

	"fyne.io/fyne/v2"
)

const OfficialManifestURL = "https://cef-builds.spotifycdn.com/index.json"

type PackageType string

const (
	PackageTypeMinimal  PackageType = "minimal"
	PackageTypeStandard PackageType = "standard"
	PackageTypeClient   PackageType = "client"
)

type BuildChannel string

const (
	ChannelStable BuildChannel = "stable"
	ChannelBeta   BuildChannel = "beta"
)

type ResourceDecision int

const (
	ResourceAllow ResourceDecision = iota
	ResourceBlock
)

type ResourceRequest struct {
	URL          string
	Method       string
	Initiator    string
	ResourceType string
	IsNavigation bool
}

type LoadProgress struct {
	URL          string
	Title        string
	Progress     float64
	IsLoading    bool
	CanGoBack    bool
	CanGoForward bool
}

type Callbacks struct {
	OnBeforeResourceLoad func(ResourceRequest) ResourceDecision
	OnLoadProgress       func(LoadProgress)
	OnAddressChange      func(string)
	OnTitleChange        func(string)
	OnError              func(error)
}

type BrowserOptions struct {
	InitialURL string
	Window     fyne.Window
	Callbacks  Callbacks
}

type Framework struct {
	Root         string
	Version      string
	Chromium     string
	Platform     string
	Channel      BuildChannel
	PackageType  PackageType
	ArchiveName  string
	ArchiveURL   string
	ArchiveSHA1  string
	ArchiveBytes int64
}

type InstallOptions struct {
	Context       context.Context
	ManifestURL   string
	Platform      string
	Channel       BuildChannel
	PackageType   PackageType
	Destination   string
	SearchPaths   []string
	HTTPClient    *http.Client
	AllowDownload bool
}

type RuntimeOptions struct {
	FrameworkRoot string
	CachePath     string
	SearchPaths   []string
	DownloadDir   string
	ManifestURL   string
	Channel       BuildChannel
	PackageType   PackageType
	AllowDownload bool
}
