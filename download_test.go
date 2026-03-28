package fynecef

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLatestBuildSelectsNewestMatchingEntry(t *testing.T) {
	var index manifest
	if err := json.Unmarshal([]byte(`{
		"linux64": {
			"versions": [
				{
					"cef_version": "144.0.17+g0ebc0d5+chromium-144.0.7559.246",
					"channel": "stable",
					"chromium_version": "144.0.7559.246",
					"files": [
						{"name":"cef_binary_linux64_client.tar.bz2","sha1":"deadbeef","size":1,"type":"client"},
						{"name":"cef_binary_linux64_minimal.tar.bz2","sha1":"cafebabe","size":2,"type":"minimal"}
					]
				},
				{
					"cef_version": "144.0.16+g1111111+chromium-144.0.7559.200",
					"channel": "stable",
					"chromium_version": "144.0.7559.200",
					"files": [
						{"name":"older_minimal.tar.bz2","sha1":"1234","size":3,"type":"minimal"}
					]
				}
			]
		}
	}`), &index); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	build, err := selectBuild(index, "https://cef-builds.spotifycdn.com/index.json", "linux64", ChannelStable, PackageTypeMinimal)
	if err != nil {
		t.Fatalf("selectBuild returned error: %v", err)
	}
	if build.Version != "144.0.17+g0ebc0d5+chromium-144.0.7559.246" {
		t.Fatalf("unexpected version: %s", build.Version)
	}
	if build.ArchiveName != "cef_binary_linux64_minimal.tar.bz2" {
		t.Fatalf("unexpected archive name: %s", build.ArchiveName)
	}
	if build.ArchiveURL != "https://cef-builds.spotifycdn.com/cef_binary_linux64_minimal.tar.bz2" {
		t.Fatalf("unexpected archive URL: %s", build.ArchiveURL)
	}
}

func TestLatestBuildSkipsIncompatibleCEFVersion(t *testing.T) {
	var index manifest
	if err := json.Unmarshal([]byte(`{
		"linux64": {
			"versions": [
				{
					"cef_version": "146.0.9+g3ca6a87+chromium-146.0.7680.165",
					"channel": "stable",
					"chromium_version": "146.0.7680.165",
					"files": [
						{"name":"cef_binary_linux64_minimal.tar.bz2","sha1":"cafebabe","size":2,"type":"minimal"}
					]
				}
			]
		}
	}`), &index); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	if _, err := selectBuild(index, "https://cef-builds.spotifycdn.com/index.json", "linux64", ChannelStable, PackageTypeMinimal); err == nil {
		t.Fatal("selectBuild expected an error for incompatible CEF version")
	}
}

func TestFindFramework(t *testing.T) {
	root := t.TempDir()
	frameworkRoot := filepath.Join(root, "cef")
	for _, dir := range []string{
		filepath.Join(frameworkRoot, "include"),
		filepath.Join(frameworkRoot, "Release"),
		filepath.Join(frameworkRoot, "Resources"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(frameworkRoot, "Release", "libcef.so"), []byte("stub"), 0o644); err != nil {
		t.Fatalf("write libcef.so: %v", err)
	}
	writeCEFVersionHeader(t, frameworkRoot, SupportedCEFVersion)

	framework, err := FindFramework(InstallOptions{SearchPaths: []string{root}})
	if err != nil {
		t.Fatalf("FindFramework returned error: %v", err)
	}
	wantRoot, err := filepath.EvalSymlinks(frameworkRoot)
	if err != nil {
		wantRoot = frameworkRoot
	}
	wantRoot, err = filepath.Abs(wantRoot)
	if err != nil {
		t.Fatalf("normalize expected root: %v", err)
	}
	if framework.Root != wantRoot {
		t.Fatalf("unexpected root: got %s want %s", framework.Root, wantRoot)
	}
}

func TestFindFrameworkHonorsRequestedPlatform(t *testing.T) {
	root := t.TempDir()
	linuxRoot := filepath.Join(root, "linux64-minimal-test")
	macRoot := filepath.Join(root, "macosx64-minimal-test")

	for _, frameworkRoot := range []string{linuxRoot, macRoot} {
		for _, dir := range []string{
			filepath.Join(frameworkRoot, "include"),
			filepath.Join(frameworkRoot, "Release"),
			filepath.Join(frameworkRoot, "Resources"),
		} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", dir, err)
			}
		}
		if err := os.WriteFile(filepath.Join(frameworkRoot, "Release", "libcef.so"), []byte("stub"), 0o644); err != nil {
			t.Fatalf("write libcef.so for %s: %v", frameworkRoot, err)
		}
		writeCEFVersionHeader(t, frameworkRoot, SupportedCEFVersion)
	}

	framework, err := FindFramework(InstallOptions{
		SearchPaths: []string{root},
		Platform:    "macosx64",
	})
	if err != nil {
		t.Fatalf("FindFramework returned error: %v", err)
	}
	if framework.Platform != "macosx64" {
		t.Fatalf("unexpected platform: got %s want macosx64", framework.Platform)
	}
}

func TestFindFrameworkSkipsIncompatibleVersion(t *testing.T) {
	root := t.TempDir()
	incompatibleRoot := filepath.Join(root, "linux64-minimal-incompatible")
	compatibleRoot := filepath.Join(root, "linux64-minimal-compatible")

	for frameworkRoot, version := range map[string]string{
		incompatibleRoot: "146.0.9+g3ca6a87+chromium-146.0.7680.165",
		compatibleRoot:   SupportedCEFVersion,
	} {
		for _, dir := range []string{
			filepath.Join(frameworkRoot, "include"),
			filepath.Join(frameworkRoot, "Release"),
			filepath.Join(frameworkRoot, "Resources"),
		} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", dir, err)
			}
		}
		if err := os.WriteFile(filepath.Join(frameworkRoot, "Release", "libcef.so"), []byte("stub"), 0o644); err != nil {
			t.Fatalf("write libcef.so for %s: %v", frameworkRoot, err)
		}
		writeCEFVersionHeader(t, frameworkRoot, version)
	}

	framework, err := FindFramework(InstallOptions{SearchPaths: []string{root}})
	if err != nil {
		t.Fatalf("FindFramework returned error: %v", err)
	}
	if framework.Version != SupportedCEFVersion {
		t.Fatalf("unexpected version: got %s want %s", framework.Version, SupportedCEFVersion)
	}
}

func TestInferFrameworkPlatform(t *testing.T) {
	tests := []struct {
		root string
		want string
	}{
		{root: filepath.Join("tmp", "linux64-minimal-123"), want: "linux64"},
		{root: filepath.Join("tmp", "macosx64-minimal-123"), want: "macosx64"},
		{root: filepath.Join("tmp", "macosarm64-minimal-123"), want: "macosarm64"},
		{root: filepath.Join("tmp", "unknown"), want: ""},
	}

	for _, test := range tests {
		if got := inferFrameworkPlatform(test.root); got != test.want {
			t.Fatalf("inferFrameworkPlatform(%q) = %q, want %q", test.root, got, test.want)
		}
	}
}

func TestPlatformName(t *testing.T) {
	tests := []struct {
		goos    string
		goarch  string
		want    string
		wantErr bool
	}{
		{goos: "linux", goarch: "amd64", want: "linux64"},
		{goos: "linux", goarch: "arm64", want: "linuxarm64"},
		{goos: "darwin", goarch: "amd64", want: "macosx64"},
		{goos: "darwin", goarch: "arm64", want: "macosarm64"},
		{goos: "windows", goarch: "amd64", want: "windows64"},
		{goos: "windows", goarch: "arm64", want: "windowsarm64"},
		{goos: "darwin", goarch: "386", wantErr: true},
	}

	for _, test := range tests {
		got, err := platformName(test.goos, test.goarch)
		if test.wantErr {
			if err == nil {
				t.Fatalf("platformName(%q, %q) expected error", test.goos, test.goarch)
			}
			continue
		}
		if err != nil {
			t.Fatalf("platformName(%q, %q) returned error: %v", test.goos, test.goarch, err)
		}
		if got != test.want {
			t.Fatalf("platformName(%q, %q) = %q, want %q", test.goos, test.goarch, got, test.want)
		}
	}
}

func writeCEFVersionHeader(t *testing.T, frameworkRoot, version string) {
	t.Helper()
	content := "#define CEF_VERSION \"" + version + "\"\n"
	if err := os.WriteFile(filepath.Join(frameworkRoot, "include", "cef_version.h"), []byte(content), 0o644); err != nil {
		t.Fatalf("write cef_version.h for %s: %v", frameworkRoot, err)
	}
}
