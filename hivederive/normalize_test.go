package hivederive

import (
	"os"
	"testing"
	"time"
)

// swapStat overrides the injectable stat function and returns a restore func.
func swapStat(fn func(string) (os.FileInfo, error)) (restore func()) {
	prev := osStat
	osStat = fn
	return func() { osStat = prev }
}

// swapRuntime overrides the injectable GOOS and WSL-marker functions and
// returns a restore func. Tests that mutate these package vars must NOT run in
// parallel.
func swapRuntime(goos string, wslMarker bool) (restore func()) {
	prevGOOS := runtimeGOOS
	prevMarker := wslMarkerFn
	runtimeGOOS = goos
	wslMarkerFn = func() bool { return wslMarker }
	return func() {
		runtimeGOOS = prevGOOS
		wslMarkerFn = prevMarker
	}
}

// TestNormalizePath_WSLGate covers Windows/WSL path translation performed only
// when the runtime is a WSL-hosted Linux daemon (GOOS=linux + WSL marker).
func TestNormalizePath_WSLGate(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"windows drive letter", `C:\Users\dev\project`, "/mnt/c/Users/dev/project"},
		{"lowercase drive letter", `d:\work\repo`, "/mnt/d/work/repo"},
		{"already posix mnt path", "/mnt/c/Users/dev/project", "/mnt/c/Users/dev/project"},
		{"unc wsl dollar", `\\wsl$\Ubuntu\home\dev\project`, "/home/dev/project"},
		{"unc wsl localhost", `\\wsl.localhost\Ubuntu\home\dev\project`, "/home/dev/project"},
		{"backslash relative path", `project\subdir`, "project/subdir"},
		{"plain posix path unchanged", "/home/dev/project", "/home/dev/project"},
	}
	restore := swapRuntime("linux", true)
	defer restore()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizePath(tt.in)
			if err != nil {
				t.Fatalf("NormalizePath(%q) error = %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("NormalizePath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestNormalizePath_NativeWindowsPassthrough proves the gate prevents a native
// Windows daemon from mistranslating a legitimate Windows path.
func TestNormalizePath_NativeWindowsPassthrough(t *testing.T) {
	restore := swapRuntime("windows", false)
	defer restore()
	in := `C:\Users\dev\project`
	got, err := NormalizePath(in)
	if err != nil {
		t.Fatalf("NormalizePath(%q) error = %v", in, err)
	}
	if got != in {
		t.Errorf("NormalizePath(%q) on native windows = %q, want unchanged", in, got)
	}
}

// TestNormalizePath_NativeLinuxPassthrough proves a native Linux daemon (no WSL
// marker) does not rewrite backslash-bearing paths, which are valid filenames.
func TestNormalizePath_NativeLinuxPassthrough(t *testing.T) {
	restore := swapRuntime("linux", false)
	defer restore()
	in := `weird\name`
	got, err := NormalizePath(in)
	if err != nil {
		t.Fatalf("NormalizePath(%q) error = %v", in, err)
	}
	if got != in {
		t.Errorf("NormalizePath(%q) on native linux = %q, want unchanged", in, got)
	}
}

// TestNormalizePath_Empty rejects an empty directory with a typed error.
func TestNormalizePath_Empty(t *testing.T) {
	if _, err := NormalizePath(""); err == nil {
		t.Fatal("NormalizePath(\"\") error = nil, want ErrEmptyDir")
	}
}

// TestDerive_NormalizesBeforeStatOnWSL proves Derive resolves a Windows-form
// path against its normalized POSIX equivalent when the raw form fails to stat.
func TestDerive_NormalizesBeforeStatOnWSL(t *testing.T) {
	restoreRT := swapRuntime("linux", true)
	defer restoreRT()

	winForm := `C:\Users\dev\myrepo`
	normalized := "/mnt/c/Users/dev/myrepo"

	// Stat succeeds only for the normalized POSIX form; the raw Windows form
	// fails, forcing the normalize-retry path in Derive.
	restoreStat := swapStat(func(p string) (os.FileInfo, error) {
		if p == normalized {
			return fakeDirInfo{}, nil
		}
		return nil, os.ErrNotExist
	})
	defer restoreStat()

	name, err := Derive(winForm)
	if err != nil {
		t.Fatalf("Derive(%q) error = %v, want nil", winForm, err)
	}
	if name != "myrepo" {
		t.Errorf("Derive(%q) = %q, want basename %q of normalized path", winForm, name, "myrepo")
	}
}

// fakeDirInfo is a minimal os.FileInfo for injected-stat tests.
type fakeDirInfo struct{}

func (fakeDirInfo) Name() string       { return "" }
func (fakeDirInfo) Size() int64        { return 0 }
func (fakeDirInfo) Mode() os.FileMode  { return os.ModeDir }
func (fakeDirInfo) ModTime() time.Time { return time.Time{} }
func (fakeDirInfo) IsDir() bool        { return true }
func (fakeDirInfo) Sys() any           { return nil }
