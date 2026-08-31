package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestFilterBuildValuesAll(t *testing.T) {
	allowed := []string{"linux", "darwin"}
	got, err := filterBuildValues("os", "all", allowed)
	if err != nil {
		t.Fatalf("filterBuildValues returned error: %v", err)
	}
	if len(got) != 2 || got[0] != "linux" || got[1] != "darwin" {
		t.Fatalf("targets = %#v, want all allowed values", got)
	}
}

func TestFilterBuildValuesSingleAndList(t *testing.T) {
	allowed := []string{"linux", "darwin", "windows"}
	got, err := filterBuildValues("os", "linux,windows,linux", allowed)
	if err != nil {
		t.Fatalf("filterBuildValues returned error: %v", err)
	}
	if len(got) != 2 || got[0] != "linux" || got[1] != "windows" {
		t.Fatalf("targets = %#v, want linux and windows once", got)
	}
}

func TestFilterBuildValuesRejectsUnknownTarget(t *testing.T) {
	_, err := filterBuildValues("arch", "amd64,unknown", []string{"amd64", "arm64"})
	if err == nil {
		t.Fatal("filterBuildValues returned nil error for an unknown target")
	}
}

func TestParseGitHubRepoRejectsForeignHostAndInvalidSlug(t *testing.T) {
	for _, remote := range []string{
		"https://example.com/owner/repo.git",
		"http://github.com/owner/repo.git",
		"git@example.com:owner/repo.git",
		"https://github.com/../repo.git",
		"https://github.com/owner/repo/extra.git",
	} {
		if _, _, err := parseGitHubRepo(remote); err == nil {
			t.Fatalf("parseGitHubRepo accepted %q", remote)
		}
	}
}

func TestParseGitHubRepoAcceptsGitHubHTTPSAndSSH(t *testing.T) {
	for _, remote := range []string{
		"https://github.com/matveynator/chicha-isotope-map.git",
		"git@github.com:matveynator/chicha-isotope-map.git",
	} {
		owner, repo, err := parseGitHubRepo(remote)
		if err != nil {
			t.Fatalf("parseGitHubRepo(%q): %v", remote, err)
		}
		if owner != "matveynator" || repo != "chicha-isotope-map" {
			t.Fatalf("parseGitHubRepo(%q) = %q/%q", remote, owner, repo)
		}
	}
}

func TestCreateReleaseArtifactsUsesGitHubReleaseNames(t *testing.T) {
	binariesPath := t.TempDir()
	serverPath := filepath.Join(binariesPath, "no-gui", "linux", "amd64", "chicha-isotope-map")
	desktopPath := filepath.Join(binariesPath, "desktop-webview", "windows", "amd64", "chicha-isotope-map.exe")
	for _, sourcePath := range []string{serverPath, desktopPath} {
		if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sourcePath, []byte("release binary"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := createReleaseArtifacts(binariesPath, "chicha-isotope-map"); err != nil {
		t.Fatal(err)
	}

	serverReleasePath := filepath.Join(binariesPath, "release", "chicha-isotope-map_linux_amd64")
	if content, err := os.ReadFile(serverReleasePath); err != nil || string(content) != "release binary" {
		t.Fatalf("server release artifact = %q, %v", content, err)
	}
	archivePath := filepath.Join(binariesPath, "release", "chicha-isotope-map_windows_amd64_desktop.zip")
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if len(archive.File) != 1 || archive.File[0].Name != "chicha-isotope-map_windows_amd64_desktop.exe" {
		t.Fatalf("desktop archive entries = %#v", archive.File)
	}
}

func TestMakeArtifactsPubliclyReadable(t *testing.T) {
	binariesPath := t.TempDir()
	artifactPath := filepath.Join(binariesPath, "version", "release", "binary")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := makeArtifactsPubliclyReadable(binariesPath); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Dir(artifactPath), artifactPath} {
		fileInfo, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fileInfo.Mode().Perm() != 0o755 {
			t.Fatalf("mode for %s = %o, want 755", path, fileInfo.Mode().Perm())
		}
	}
}
