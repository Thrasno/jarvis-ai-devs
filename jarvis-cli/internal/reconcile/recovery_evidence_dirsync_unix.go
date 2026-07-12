//go:build !windows

package reconcile

import "os"

// syncRecoveryEvidenceDirectory flushes the renamed directory entry. The
// evidence file is already replaced when this fails, but callers must fail
// closed because its crash durability is uncertain.
func syncRecoveryEvidenceDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
