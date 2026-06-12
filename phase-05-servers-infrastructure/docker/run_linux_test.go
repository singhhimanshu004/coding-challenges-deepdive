//go:build linux

package main

// run_linux_test.go holds the LINUX-ONLY integration tests. The `//go:build
// linux` tag means the Go toolchain skips this file entirely on macOS, so the
// default `go test ./...` on the Mac dev machine stays green while still
// exercising the platform-neutral tests. On Linux these run, and the ones that
// need real isolation skip themselves unless executed as root.

import (
	"bytes"
	"os"
	"testing"
)

// TestRunParentNeedsPrivileges checks that launching a container against a
// non-existent rootfs fails rather than silently "succeeding". Creating new
// namespaces requires root (or a user namespace), so when not root we skip.
func TestRunParentNeedsPrivileges(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root to create namespaces; run with sudo to exercise")
	}

	cfg := &Config{
		Hostname: "test",
		RootFS:   "/nonexistent-rootfs-for-gocker-test",
		Command:  "/bin/true",
	}

	var out, errBuf bytes.Buffer
	if err := runParent(cfg, &out, &errBuf); err == nil {
		t.Error("expected error launching container with a missing rootfs")
	}
}

// TestPivotRootRejectsMissingRoot confirms pivotRoot surfaces an error for a
// path that cannot be made a mount point. Also gated on root.
func TestPivotRootRejectsMissingRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root to manipulate mounts")
	}
	if err := pivotRoot("/definitely/not/here"); err == nil {
		t.Error("expected pivotRoot to fail for a missing directory")
	}
}
