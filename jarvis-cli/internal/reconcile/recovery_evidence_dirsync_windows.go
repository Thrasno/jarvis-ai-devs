//go:build windows

package reconcile

// syncRecoveryEvidenceDirectory is intentionally a no-op on Windows: Go does
// not provide a portable, supported directory-sync operation there. Rename
// remains atomic where Windows supports it, and the caller retains sanitized
// failure handling for every operation it can verify.
func syncRecoveryEvidenceDirectory(string) error {
	return nil
}
