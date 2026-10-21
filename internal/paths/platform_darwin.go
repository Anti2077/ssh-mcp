//go:build darwin

package paths

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func platformDefaultRoots(executable string) (Roots, error) {
	configDir := filepath.Dir(executable)
	runtimeDir := filepath.Join(configDir, runtimeDirectoryName)
	// Both endpoints must fit sun_path, including its terminating NUL. Keep
	// the existing layout for short installations and already-running daemons.
	if len(filepath.Join(runtimeDir, "control.sock")) < len(syscall.RawSockaddrUnix{}.Path) {
		return Prepare(configDir, runtimeDir)
	}
	if err := EnsureDirectory(configDir); err != nil {
		return Roots{}, fmt.Errorf("prepare config directory: %w", err)
	}
	canonicalDir, err := filepath.EvalSymlinks(configDir)
	if err != nil {
		return Roots{}, fmt.Errorf("resolve config directory: %w", err)
	}
	canonicalDir, err = filepath.Abs(canonicalDir)
	if err != nil {
		return Roots{}, fmt.Errorf("resolve absolute config directory: %w", err)
	}
	// TMPDIR on macOS can itself exceed the socket path limit. /private/tmp
	// is short and stable across the bridge, daemon and local console.
	digest := sha256.Sum256([]byte(canonicalDir))
	runtimeDir = filepath.Join("/private/tmp", fmt.Sprintf("ssh-mcp-%d-%x", os.Getuid(), digest[:16]))
	if err := ensurePrivateRuntimeDirectory(runtimeDir); err != nil {
		return Roots{}, fmt.Errorf("prepare short runtime directory: %w", err)
	}
	return Roots{ConfigDir: configDir, RuntimeDir: runtimeDir}, nil
}

func ensurePrivateRuntimeDirectory(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("create directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("runtime directory must be a real directory")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) {
		return fmt.Errorf("runtime directory must be owned by the current user")
	}
	if info.Mode().Perm() != 0o700 {
		return fmt.Errorf("runtime directory must have permissions 0700")
	}
	return nil
}
