package fynecef

import (
	"archive/tar"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"compress/bzip2"
)

type manifest struct {
	Platforms map[string]manifestPlatform
}

func (m *manifest) UnmarshalJSON(data []byte) error {
	var raw map[string]manifestPlatform
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.Platforms = raw
	return nil
}

type manifestPlatform struct {
	Versions []manifestVersion `json:"versions"`
}

type manifestVersion struct {
	CEFVersion      string         `json:"cef_version"`
	Channel         string         `json:"channel"`
	ChromiumVersion string         `json:"chromium_version"`
	Files           []manifestFile `json:"files"`
}

type manifestFile struct {
	LastModified time.Time `json:"last_modified"`
	Name         string    `json:"name"`
	SHA1         string    `json:"sha1"`
	Size         int64     `json:"size"`
	Type         string    `json:"type"`
}

func LatestBuild(ctx context.Context, opts InstallOptions) (*Framework, error) {
	ctx = fallbackContext(ctx, opts.Context)
	manifestURL := opts.ManifestURL
	if manifestURL == "" {
		manifestURL = OfficialManifestURL
	}

	platform := opts.Platform
	if platform == "" {
		var err error
		platform, err = currentPlatform()
		if err != nil {
			return nil, err
		}
	}

	channel := opts.Channel
	if channel == "" {
		channel = ChannelStable
	}

	pkgType := opts.PackageType
	if pkgType == "" {
		pkgType = PackageTypeMinimal
	}

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fynecef: fetch manifest: unexpected status %s", resp.Status)
	}

	var index manifest
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, fmt.Errorf("fynecef: decode manifest: %w", err)
	}

	return selectBuild(index, manifestURL, platform, channel, pkgType)
}

func selectBuild(index manifest, manifestURL, platform string, channel BuildChannel, pkgType PackageType) (*Framework, error) {
	entry, ok := index.Platforms[platform]
	if !ok {
		return nil, fmt.Errorf("%w for platform %q", ErrNoBuild, platform)
	}

	for _, version := range entry.Versions {
		if BuildChannel(version.Channel) != channel {
			continue
		}
		if version.CEFVersion != SupportedCEFVersion {
			continue
		}
		for _, file := range version.Files {
			if PackageType(file.Type) != pkgType {
				continue
			}
			archiveURL, err := resolveArchiveURL(manifestURL, file.Name)
			if err != nil {
				return nil, err
			}
			return &Framework{
				Version:      version.CEFVersion,
				Chromium:     version.ChromiumVersion,
				Platform:     platform,
				Channel:      channel,
				PackageType:  pkgType,
				ArchiveName:  file.Name,
				ArchiveURL:   archiveURL,
				ArchiveSHA1:  file.SHA1,
				ArchiveBytes: file.Size,
			}, nil
		}
	}

	return nil, fmt.Errorf("%w for platform=%s channel=%s type=%s cef_version=%s", ErrNoBuild, platform, channel, pkgType, SupportedCEFVersion)
}

func FindFramework(opts InstallOptions) (*Framework, error) {
	incompatibleFound := false
	paths := append([]string{}, opts.SearchPaths...)
	if opts.Destination != "" {
		paths = append(paths, opts.Destination)
	}
	for _, root := range paths {
		if root == "" {
			continue
		}
		if framework, ok := inspectFramework(root); ok {
			if !isCompatibleFrameworkVersion(framework.Version) {
				incompatibleFound = true
				continue
			}
			if opts.Platform != "" && framework.Platform != "" && framework.Platform != opts.Platform {
				continue
			}
			return framework, nil
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if framework, ok := inspectFramework(filepath.Join(root, entry.Name())); ok {
				if !isCompatibleFrameworkVersion(framework.Version) {
					incompatibleFound = true
					continue
				}
				if opts.Platform != "" && framework.Platform != "" && framework.Platform != opts.Platform {
					continue
				}
				return framework, nil
			}
		}
	}
	if incompatibleFound {
		return nil, fmt.Errorf("%w: require CEF %s", ErrNoFramework, SupportedCEFVersion)
	}
	return nil, ErrNoFramework
}

func EnsureFramework(opts InstallOptions) (*Framework, error) {
	if framework, err := FindFramework(opts); err == nil {
		return framework, nil
	}
	if !opts.AllowDownload {
		return nil, ErrNoFramework
	}

	build, err := LatestBuild(opts.Context, opts)
	if err != nil {
		return nil, err
	}

	destination := opts.Destination
	if destination == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		destination = filepath.Join(cacheDir, "fynecef", build.Platform, string(build.PackageType))
	}

	if err := os.MkdirAll(destination, 0o755); err != nil {
		return nil, err
	}

	installRoot := filepath.Join(destination, safeFrameworkDir(build))
	if framework, ok := inspectFramework(installRoot); ok {
		framework.Version = build.Version
		framework.Chromium = build.Chromium
		framework.Channel = build.Channel
		framework.PackageType = build.PackageType
		framework.Platform = build.Platform
		framework.ArchiveName = build.ArchiveName
		framework.ArchiveURL = build.ArchiveURL
		framework.ArchiveSHA1 = build.ArchiveSHA1
		framework.ArchiveBytes = build.ArchiveBytes
		return framework, nil
	}

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 0}
	}

	if err := downloadAndExtract(fallbackContext(opts.Context, context.Background()), client, build, installRoot); err != nil {
		return nil, err
	}

	framework, ok := inspectFramework(installRoot)
	if !ok {
		return nil, fmt.Errorf("fynecef: extracted framework is incomplete at %s", installRoot)
	}
	framework.Version = build.Version
	framework.Chromium = build.Chromium
	framework.Channel = build.Channel
	framework.PackageType = build.PackageType
	framework.Platform = build.Platform
	framework.ArchiveName = build.ArchiveName
	framework.ArchiveURL = build.ArchiveURL
	framework.ArchiveSHA1 = build.ArchiveSHA1
	framework.ArchiveBytes = build.ArchiveBytes
	return framework, nil
}

func LinkCurrentFramework(linkPath string, framework *Framework) error {
	if framework == nil {
		return ErrNoFramework
	}
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		return err
	}

	info, err := os.Lstat(linkPath)
	switch {
	case err == nil && info.Mode()&os.ModeSymlink != 0:
		if err := os.Remove(linkPath); err != nil {
			return err
		}
	case err == nil:
		return fmt.Errorf("fynecef: refusing to replace non-symlink path %s", linkPath)
	case os.IsNotExist(err):
	default:
		return err
	}

	linkDir, err := filepath.Abs(filepath.Dir(linkPath))
	if err != nil {
		return err
	}
	frameworkRoot, err := filepath.Abs(framework.Root)
	if err != nil {
		return err
	}
	target, err := filepath.Rel(linkDir, frameworkRoot)
	if err != nil {
		return err
	}
	return os.Symlink(target, linkPath)
}

func currentPlatform() (string, error) {
	return platformName(runtime.GOOS, runtime.GOARCH)
}

func platformName(goos, goarch string) (string, error) {
	switch goos + "/" + goarch {
	case "linux/amd64":
		return "linux64", nil
	case "linux/arm64":
		return "linuxarm64", nil
	case "darwin/amd64":
		return "macosx64", nil
	case "darwin/arm64":
		return "macosarm64", nil
	case "windows/amd64":
		return "windows64", nil
	case "windows/arm64":
		return "windowsarm64", nil
	}
	return "", fmt.Errorf("fynecef: unsupported platform %s/%s", goos, goarch)
}

func resolveArchiveURL(manifestURL, archiveName string) (string, error) {
	base, err := url.Parse(manifestURL)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(base.Path, "/") {
		idx := strings.LastIndex(base.Path, "/")
		if idx >= 0 {
			base.Path = base.Path[:idx+1]
		}
	}
	ref, err := url.Parse(archiveName)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}

func downloadAndExtract(ctx context.Context, client *http.Client, build *Framework, installRoot string) error {
	tmpDir := installRoot + ".tmp"
	_ = os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, build.ArchiveURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fynecef: download archive: unexpected status %s", resp.Status)
	}

	hash := sha1.New()
	stream := io.TeeReader(resp.Body, hash)
	bz2 := bzip2.NewReader(stream)
	tarReader := tar.NewReader(bz2)

	var extractedTop string
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		name := filepath.Clean(header.Name)
		if name == "." || strings.HasPrefix(name, "..") {
			continue
		}

		parts := strings.Split(name, string(filepath.Separator))
		if extractedTop == "" && len(parts) > 0 {
			extractedTop = parts[0]
		}

		target := filepath.Join(tmpDir, name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, tarReader); err != nil {
				file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		}
	}

	if build.ArchiveSHA1 != "" {
		sum := hex.EncodeToString(hash.Sum(nil))
		if !strings.EqualFold(sum, build.ArchiveSHA1) {
			return fmt.Errorf("fynecef: archive sha1 mismatch: got %s want %s", sum, build.ArchiveSHA1)
		}
	}

	sourceRoot := tmpDir
	if extractedTop != "" {
		sourceRoot = filepath.Join(tmpDir, extractedTop)
	}
	if _, err := os.Stat(sourceRoot); err != nil {
		return err
	}

	if err := os.RemoveAll(installRoot); err != nil {
		return err
	}
	return os.Rename(sourceRoot, installRoot)
}

func inspectFramework(root string) (*Framework, bool) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		root = filepath.Clean(root)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, false
	}
	for _, path := range []string{
		filepath.Join(root, "include"),
		filepath.Join(root, "Release"),
	} {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return nil, false
		}
	}

	platform := inferFrameworkPlatform(root)
	switch {
	case hasFrameworkLayout(root, "libcef.so", filepath.Join("Resources")):
		if platform == "" {
			platform = "linux64"
		}
	case hasFrameworkLayout(root, "libcef.dll", filepath.Join("Resources")):
		if platform == "" {
			platform = "windows64"
		}
	case hasMacFrameworkLayout(root):
		if platform == "" {
			platform = "macosx64"
		}
	default:
		return nil, false
	}

	return &Framework{
		Root:     root,
		Version:  readFrameworkVersion(root),
		Platform: platform,
	}, true
}

func readFrameworkVersion(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "include", "cef_version.h"))
	if err != nil {
		return ""
	}
	const prefix = "#define CEF_VERSION \""
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		version, _, ok := strings.Cut(strings.TrimPrefix(line, prefix), "\"")
		if ok {
			return version
		}
	}
	return ""
}

func isCompatibleFrameworkVersion(version string) bool {
	return version == SupportedCEFVersion
}

func hasFrameworkLayout(root, releaseLib string, resourcesPath string) bool {
	if _, err := os.Stat(filepath.Join(root, "Release", releaseLib)); err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(root, resourcesPath))
	return err == nil && info.IsDir()
}

func hasMacFrameworkLayout(root string) bool {
	frameworkDir := filepath.Join(root, "Release", "Chromium Embedded Framework.framework")
	if info, err := os.Stat(frameworkDir); err != nil || !info.IsDir() {
		return false
	}
	info, err := os.Stat(filepath.Join(frameworkDir, "Resources"))
	return err == nil && info.IsDir()
}

func inferFrameworkPlatform(root string) string {
	root = filepath.Clean(root)
	parts := strings.Split(root, string(filepath.Separator))
	for _, part := range parts {
		for _, platform := range []string{
			"linux64",
			"linuxarm64",
			"macosx64",
			"macosarm64",
			"windows64",
			"windowsarm64",
		} {
			if part == platform || strings.HasPrefix(part, platform+"-") {
				return platform
			}
		}
	}
	return ""
}

func safeFrameworkDir(build *Framework) string {
	version := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(build.Version)
	return fmt.Sprintf("%s-%s-%s", build.Platform, build.PackageType, version)
}

func fallbackContext(ctx context.Context, fallback context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	if fallback != nil {
		return fallback
	}
	return context.Background()
}
