//go:build linux

package main

// cgroup_linux.go implements the resource-limiting half of the runtime using
// CONTROL GROUPS (cgroups) — the kernel feature that caps how much memory, CPU,
// or how many processes a group of tasks may use. Namespaces decide what a
// process can SEE; cgroups decide how much it can USE. Together they are the two
// pillars every real container runtime stands on.
//
// cgroups are driven entirely through a pseudo-filesystem under /sys/fs/cgroup:
// you make a directory (that creates a group), write numbers into control files
// (the limits), and write a PID into cgroup.procs (join the group). There are
// two incompatible layouts in the wild:
//
//   - cgroup v2 (unified, modern default): one tree, files like memory.max,
//     pids.max.
//   - cgroup v1 (legacy): a separate tree per controller, files like
//     memory.limit_in_bytes, pids.max.
//
// We detect which one is mounted and write the right files.

import (
	"os"
	"path/filepath"
	"strconv"
)

const cgroupRoot = "/sys/fs/cgroup"

// applyCgroups places the current process (os.Getpid) into a "gocker" cgroup
// with the requested limits. A zero limit means "leave unlimited".
func applyCgroups(cfg *Config) error {
	if cfg.MemoryLimit == 0 && cfg.PidsLimit == 0 {
		return nil // nothing to constrain
	}
	if isCgroupV2() {
		return applyCgroupV2(cfg)
	}
	return applyCgroupV1(cfg)
}

// isCgroupV2 reports whether the unified (v2) hierarchy is mounted. The presence
// of /sys/fs/cgroup/cgroup.controllers is the canonical v2 marker.
func isCgroupV2() bool {
	_, err := os.Stat(filepath.Join(cgroupRoot, "cgroup.controllers"))
	return err == nil
}

func applyCgroupV2(cfg *Config) error {
	dir := filepath.Join(cgroupRoot, "gocker")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if cfg.MemoryLimit > 0 {
		if err := writeCgroupFile(dir, "memory.max", strconv.FormatInt(cfg.MemoryLimit, 10)); err != nil {
			return err
		}
	}
	if cfg.PidsLimit > 0 {
		if err := writeCgroupFile(dir, "pids.max", strconv.Itoa(cfg.PidsLimit)); err != nil {
			return err
		}
	}
	// Writing our PID into cgroup.procs makes THIS process (and its future
	// children, e.g. the container command) members of the group.
	return writeCgroupFile(dir, "cgroup.procs", strconv.Itoa(os.Getpid()))
}

func applyCgroupV1(cfg *Config) error {
	pid := strconv.Itoa(os.Getpid())

	if cfg.MemoryLimit > 0 {
		dir := filepath.Join(cgroupRoot, "memory", "gocker")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := writeCgroupFile(dir, "memory.limit_in_bytes", strconv.FormatInt(cfg.MemoryLimit, 10)); err != nil {
			return err
		}
		if err := writeCgroupFile(dir, "cgroup.procs", pid); err != nil {
			return err
		}
	}

	if cfg.PidsLimit > 0 {
		dir := filepath.Join(cgroupRoot, "pids", "gocker")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := writeCgroupFile(dir, "pids.max", strconv.Itoa(cfg.PidsLimit)); err != nil {
			return err
		}
		if err := writeCgroupFile(dir, "cgroup.procs", pid); err != nil {
			return err
		}
	}
	return nil
}

func writeCgroupFile(dir, name, data string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644)
}
