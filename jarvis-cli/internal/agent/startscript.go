package agent

import (
	"os"
	"path/filepath"
	"runtime"
)

// HiveDaemonBinaryPath returns the expected path to the hive-daemon binary.
// It uses $GOPATH/bin if GOPATH is set, otherwise falls back to ~/go/bin.
// On Windows the binary has a .exe suffix.
func HiveDaemonBinaryPath(home string) string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = filepath.Join(home, "go")
	}
	name := "hive-daemon"
	if runtime.GOOS == "windows" {
		name = "hive-daemon.exe"
	}
	return filepath.Join(gopath, "bin", name)
}
