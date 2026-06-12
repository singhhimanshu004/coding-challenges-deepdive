package main

// layers.go is PLATFORM-NEUTRAL. It computes the directory layout for an
// OverlayFS-based root filesystem. The PATH MATH here is pure string/path
// logic, so it is unit-tested on macOS even though the actual `mount -t overlay`
// can only happen on Linux (see run_linux.go).
//
// WHY OVERLAYFS? A real Docker image is a STACK of read-only layers (the base
// OS, then "apt install nginx", then "copy my app", …). OverlayFS lets the
// kernel present that stack PLUS a writable scratch layer as a single merged
// directory, without copying gigabytes around. The container writes to the top
// writable layer; the read-only layers underneath are shared between every
// container that uses the same image. That copy-on-write sharing is why
// launching the 100th container off an image is nearly free.

import (
	"path/filepath"
	"strings"
)

// OverlayLayout is the resolved set of directories OverlayFS needs.
//
//	lowerdir : the read-only image layers (highest-priority first)
//	upperdir : the writable layer — all changes the container makes land here
//	workdir  : scratch space OverlayFS needs internally (must be empty, same fs)
//	merged   : the unified view that becomes the container's "/"
type OverlayLayout struct {
	Lower  []string
	Upper  string
	Work   string
	Merged string
}

// BuildOverlayLayout computes the layout for a container whose read-only image
// is composed of lowerLayers and whose private scratch space lives under
// containerDir.
//
// Ordering gotcha (the reason this needs its own tested function): OverlayFS
// reads lowerdir as HIGHEST-priority FIRST, but images are conventionally
// listed BASE-first (oldest → newest). So we reverse the list: the newest image
// layer must win when the same file exists in several layers.
func BuildOverlayLayout(containerDir string, lowerLayers []string) OverlayLayout {
	lower := make([]string, len(lowerLayers))
	for i, l := range lowerLayers {
		lower[len(lowerLayers)-1-i] = filepath.Clean(l)
	}

	return OverlayLayout{
		Lower:  lower,
		Upper:  filepath.Join(containerDir, "diff"),
		Work:   filepath.Join(containerDir, "work"),
		Merged: filepath.Join(containerDir, "merged"),
	}
}

// MountOptions renders the layout as the comma-separated option string passed to
// the mount(2) syscall / `mount -t overlay -o ...`. The produced string looks
// like:
//
//	lowerdir=/img/top:/img/base,upperdir=/c/diff,workdir=/c/work
func (o OverlayLayout) MountOptions() string {
	return strings.Join([]string{
		"lowerdir=" + strings.Join(o.Lower, ":"),
		"upperdir=" + o.Upper,
		"workdir=" + o.Work,
	}, ",")
}
