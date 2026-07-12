package main

import "testing"

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
