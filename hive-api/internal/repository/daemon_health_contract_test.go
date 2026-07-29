package repository

import (
	"os"
	"strings"
	"testing"
)

func TestSyncAttemptRepositoryDoesNotRetainDaemonHealthContract(t *testing.T) {
	t.Helper()

	for _, file := range []string{
		"sync_attempt.go",
		"postgres_sync_attempt.go",
		"mock_sync_attempt.go",
	} {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if strings.Contains(string(content), "DaemonHealth") {
			t.Errorf("%s retains the removed DaemonHealth contract", file)
		}
	}
}
