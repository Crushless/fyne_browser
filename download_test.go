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

	framework, err := FindFramework(InstallOptions{SearchPaths: []string{root}})
	if err != nil {
		t.Fatalf("FindFramework returned error: %v", err)
	}
	if framework.Root != frameworkRoot {
		t.Fatalf("unexpected root: %s", framework.Root)
	}
}
