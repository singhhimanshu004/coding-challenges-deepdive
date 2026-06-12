//go:build linux

package main

// run_linux.go is the REAL container runtime. It is compiled ONLY on Linux
// (see the `//go:build linux` tag above), because everything here calls Linux
// kernel features that simply do not exist on macOS or Windows:
//
//   - syscall.SysProcAttr.Cloneflags  → new namespaces (clone(2) flags)
//   - syscall.Sethostname             → UTS namespace
//   - syscall.PivotRoot / Chroot      → root filesystem isolation
//   - syscall.Mount                   → fresh /proc, private mounts
//   - the cgroup pseudo-filesystem    → resource limits (see cgroup_linux.go)
//
// The flow has two halves connected by the RE-EXEC TRICK (explained in main.go):
//
//   runParent  → clone() into new namespaces, re-exec ourselves as `child`
//   runChild   → (now inside the namespaces) hostname, cgroups, pivot_root,
//                mount /proc, then finally run the user's command.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// runParent handles `gocker run ...`. It does NOT set up the container itself;
// it asks the kernel to create new namespaces and then re-launches THIS binary
// (`/proc/self/exe`) in child mode, where the real setup happens.
func runParent(cfg *Config, stdout, stderr io.Writer) error {
	fmt.Fprintf(stderr, "gocker: launching container (rootfs=%q cmd=%q mem=%d pids=%d)\n",
		cfg.RootFS, cfg.Command, cfg.MemoryLimit, cfg.PidsLimit)

	// /proc/self/exe is a kernel-provided symlink to the currently running
	// executable. Re-executing it is how we "fork into" the new namespaces
	// with the Go runtime intact.
	cmd := exec.Command("/proc/self/exe", childArgs(cfg)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	// 🐍 SysProcAttr is Go's per-platform "extra knobs for the new process".
	// Cloneflags map 1:1 onto the flags of the Linux clone(2) syscall. Each
	// CLONE_NEW* asks for a brand-new instance of one kind of namespace:
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | // own hostname/domainname
			syscall.CLONE_NEWPID | // own PID tree (its first proc is PID 1)
			syscall.CLONE_NEWNS | // own mount table
			syscall.CLONE_NEWNET | // own (empty) network stack
			syscall.CLONE_NEWIPC, // own SysV IPC / POSIX msg queues

		// Make the child's mount namespace PRIVATE so the bind/pivot mounts we
		// do inside never propagate back to the host's mount table.
		Unshareflags: syscall.CLONE_NEWNS,

		// --- USER NAMESPACE (optional, advanced) ------------------------------
		// Adding CLONE_NEWUSER lets an UNPRIVILEGED user run gocker by mapping
		// container-root (uid 0) to your real uid on the host. It is powerful
		// but interacts awkwardly with mounting /proc and CLONE_NEWNET on some
		// kernels, so it is left commented for clarity. To enable, OR in
		// syscall.CLONE_NEWUSER above and uncomment:
		//
		// UidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		// GidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("container exited: %w", err)
	}
	return nil
}

// runChild executes INSIDE the new namespaces (we are PID 1 of the new PID
// namespace here). This is where the container actually takes shape.
func runChild(cfg *Config, stdout, stderr io.Writer) error {
	// 1) Hostname — visible because we are in a fresh UTS namespace; setting it
	//    here does NOT change the host's hostname.
	if err := syscall.Sethostname([]byte(cfg.Hostname)); err != nil {
		return fmt.Errorf("sethostname: %w", err)
	}

	// 2) Resource limits via cgroups (memory / pids). Done before we run the
	//    command so the command is born already constrained.
	if err := applyCgroups(cfg); err != nil {
		return fmt.Errorf("cgroups: %w", err)
	}

	// 3) Root filesystem isolation: make cfg.RootFS the new "/".
	if err := pivotRoot(cfg.RootFS); err != nil {
		return fmt.Errorf("pivot_root: %w", err)
	}

	// 4) A fresh /proc so tools like `ps` see only the container's processes
	//    (this needs the new PID + mount namespaces we already created).
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("mount /proc: %w", err)
	}
	defer func() { _ = syscall.Unmount("/proc", 0) }()

	// 5) Finally, become the user's command. We keep the Go process as PID 1
	//    and run the command as its child via os/exec (simple and robust). A
	//    "true" runtime might syscall.Exec to replace itself entirely.
	proc := exec.Command(cfg.Command, cfg.Args...)
	proc.Stdin = os.Stdin
	proc.Stdout = stdout
	proc.Stderr = stderr
	proc.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/",
		"TERM=xterm",
	}
	if err := proc.Run(); err != nil {
		return fmt.Errorf("running %q: %w", cfg.Command, err)
	}
	return nil
}

// pivotRoot swaps the process's root filesystem to newRoot. It is the safer,
// modern alternative to chroot(2): chroot only changes "/" lookups and can be
// escaped, while pivot_root genuinely detaches the old root so the container
// can no longer reach the host filesystem.
//
// The dance below is the canonical recipe:
//  1. make the whole mount tree private (so nothing leaks to the host),
//  2. bind-mount newRoot onto itself (pivot_root requires new_root to be a
//     mount point),
//  3. pivot_root(newRoot, putOld) — newRoot becomes "/", old root moves under
//     putOld,
//  4. chdir("/"), then detach and remove the old root.
func pivotRoot(newRoot string) error {
	newRoot, err := filepath.Abs(newRoot)
	if err != nil {
		return err
	}

	// 1) Recursively make every mount private to this namespace.
	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("make / private: %w", err)
	}

	// 2) Bind-mount newRoot onto itself so it qualifies as a mount point.
	if err := syscall.Mount(newRoot, newRoot, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind new root: %w", err)
	}

	// 3) Move the current root under newRoot/.pivot_old.
	putOld := filepath.Join(newRoot, ".pivot_old")
	if err := os.MkdirAll(putOld, 0o700); err != nil {
		return fmt.Errorf("mkdir .pivot_old: %w", err)
	}
	if err := syscall.PivotRoot(newRoot, putOld); err != nil {
		return fmt.Errorf("pivot_root syscall: %w", err)
	}

	// 4) Now "/" is the new root; move into it and discard the old one.
	if err := syscall.Chdir("/"); err != nil {
		return fmt.Errorf("chdir /: %w", err)
	}
	const oldRoot = "/.pivot_old"
	if err := syscall.Unmount(oldRoot, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("unmount old root: %w", err)
	}
	return os.Remove(oldRoot)
}
