package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	fynecef "fyne_browser"
)

func main() {
	var (
		dest     = flag.String("dest", filepath.Join("third_party", "cef", "builds"), "directory to store downloaded CEF builds")
		channel  = flag.String("channel", string(fynecef.ChannelStable), "release channel to install: stable or beta")
		pkgType  = flag.String("package", string(fynecef.PackageTypeMinimal), "package type to install: minimal, standard or client")
		platform = flag.String("platform", "", "target CEF platform, for example linux64, macosx64, macosarm64, windows64")
		linkPath = flag.String("link", filepath.Join("third_party", "cef", "current"), "stable symlink path used by local builds")
		manifest = flag.String("manifest", fynecef.OfficialManifestURL, "CEF manifest URL")
	)
	flag.Parse()

	framework, err := fynecef.EnsureFramework(fynecef.InstallOptions{
		Context:       context.Background(),
		ManifestURL:   *manifest,
		Platform:      *platform,
		Channel:       fynecef.BuildChannel(*channel),
		PackageType:   fynecef.PackageType(*pkgType),
		Destination:   *dest,
		AllowDownload: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap failed: %v\n", err)
		os.Exit(1)
	}
	if err := fynecef.LinkCurrentFramework(*linkPath, framework); err != nil {
		fmt.Fprintf(os.Stderr, "link current framework failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("installed CEF %s (%s %s) at %s\n", framework.Version, framework.Platform, framework.PackageType, framework.Root)
	fmt.Printf("current link updated at %s\n", *linkPath)
}
