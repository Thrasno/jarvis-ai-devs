package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// HiveDaemonBinaryPath resolves the hive-daemon binary location.
//
// Resolution order:
//  1. installer-managed location (/usr/local/bin on Unix,
//     %LOCALAPPDATA%\Programs\jarvis on Windows)
//  2. executable available in PATH
//  3. legacy Go install locations ($GOPATH/bin or ~/go/bin)
//
// On Windows the binary has a .exe suffix.
func HiveDaemonBinaryPath(home string) string {
	name := "hive-daemon"
	if runtime.GOOS == "windows" {
		name = "hive-daemon.exe"
	}

	installerPath := installerManagedHivePath(name)
	if isExecutableBinary(installerPath) {
		return installerPath
	}

	if resolved, err := exec.LookPath(name); err == nil {
		if isExecutableBinary(resolved) {
			return resolved
		}
	}

	candidates := make([]string, 0, 2)

	gopath := os.Getenv("GOPATH")
	if gopath != "" {
		candidates = append(candidates, filepath.Join(gopath, "bin", name))
	}
	candidates = append(candidates, filepath.Join(home, "go", "bin", name))

	for _, candidate := range candidates {
		if isExecutableBinary(candidate) {
			return candidate
		}
	}

	return installerPath
}

func installerManagedHivePath(binaryName string) string {
	if runtime.GOOS == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "Programs", "jarvis", binaryName)
		}
	}
	return filepath.Join("/usr/local/bin", binaryName)
}

func isExecutableBinary(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return fi.Mode().Perm()&0o111 != 0
}
