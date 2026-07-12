package agent

import "os"

func validHiveDaemonPath(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && validHiveDaemonExecutable(path, info)
}
