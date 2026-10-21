//go:build darwin || windows

package paths

import (
	"path/filepath"
	"testing"
)

func TestUserPlatformRootsUseExecutableDirectory(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "app", "ssh-mcp.exe")
	roots, err := defaultRoots(executable)
	if err != nil {
		t.Fatalf("defaultRoots() error = %v", err)
	}
	if want := filepath.Dir(executable); roots.ConfigDir != want {
		t.Errorf("ConfigDir = %q, want executable directory %q", roots.ConfigDir, want)
	}
	assertDefaultRuntimeDirectory(t, executable, roots.RuntimeDir)
	assertDirectory(t, roots.ConfigDir)
	assertDirectory(t, roots.RuntimeDir)
}
