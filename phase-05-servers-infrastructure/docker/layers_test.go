package main

// layers_test.go is PLATFORM-NEUTRAL: it verifies the OverlayFS path math, which
// is the part of the layered-filesystem story we can test without a Linux
// kernel. Runs on macOS in CI.

import (
	"reflect"
	"testing"
)

func TestBuildOverlayLayout(t *testing.T) {
	// Images are listed base-first: base → middle → top.
	got := BuildOverlayLayout("/var/lib/gocker/c1", []string{
		"/img/base",
		"/img/middle",
		"/img/top",
	})

	// OverlayFS wants highest-priority FIRST, so the order must be reversed.
	wantLower := []string{"/img/top", "/img/middle", "/img/base"}
	if !reflect.DeepEqual(got.Lower, wantLower) {
		t.Errorf("Lower = %v, want %v", got.Lower, wantLower)
	}
	if got.Upper != "/var/lib/gocker/c1/diff" {
		t.Errorf("Upper = %q", got.Upper)
	}
	if got.Work != "/var/lib/gocker/c1/work" {
		t.Errorf("Work = %q", got.Work)
	}
	if got.Merged != "/var/lib/gocker/c1/merged" {
		t.Errorf("Merged = %q", got.Merged)
	}
}

func TestOverlayMountOptions(t *testing.T) {
	layout := BuildOverlayLayout("/c", []string{"/img/base", "/img/top"})
	want := "lowerdir=/img/top:/img/base,upperdir=/c/diff,workdir=/c/work"
	if got := layout.MountOptions(); got != want {
		t.Errorf("MountOptions() =\n  %q\nwant\n  %q", got, want)
	}
}

func TestBuildOverlayLayoutCleansPaths(t *testing.T) {
	got := BuildOverlayLayout("/c", []string{"/img/base/", "/img/../img/top"})
	want := []string{"/img/top", "/img/base"}
	if !reflect.DeepEqual(got.Lower, want) {
		t.Errorf("Lower = %v, want %v (paths should be cleaned)", got.Lower, want)
	}
}
