//go:build !windows

package agent

import "io/fs"

func validHiveDaemonExecutable(_ string, info fs.FileInfo) bool {
	return info.Mode().Perm()&0o111 != 0
}
