//go:build windows

package agent

import (
	"io/fs"
	"path/filepath"
	"strings"
)

func validHiveDaemonExecutable(path string, _ fs.FileInfo) bool {
	return strings.EqualFold(filepath.Ext(path), ".exe")
}
