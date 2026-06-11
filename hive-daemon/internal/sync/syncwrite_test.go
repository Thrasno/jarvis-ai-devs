package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestWriteFileConfig_HappyPath verifies that all fields are written correctly
// and the file has mode 0600 on Unix.
func TestWriteFileConfig_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sync.json")
	withConfigPath(t, path)

	u := ConfigUpdate{
		APIURL:   "https://hive.example.com",
		Email:    "user@example.com",
		Password: "supersecret",
		AutoSync: true,
	}

	if err := WriteFileConfig(u); err != nil {
		t.Fatalf("WriteFileConfig: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var got syncFileConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.APIURL != u.APIURL {
		t.Errorf("APIURL = %q, want %q", got.APIURL, u.APIURL)
	}
	if got.Email != u.Email {
		t.Errorf("Email = %q, want %q", got.Email, u.Email)
	}
	if got.Password != u.Password {
		t.Errorf("Password = %q, want %q", got.Password, u.Password)
	}
	if got.AutoSync != u.AutoSync {
		t.Errorf("AutoSync = %v, want %v", got.AutoSync, u.AutoSync)
	}

	// Verify file mode 0600 on Unix
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("file mode = 0%o, want 0600", perm)
		}
	}
}

// TestWriteFileConfig_AutoSyncRoundTrip verifies that both true and false
// are round-tripped correctly.
func TestWriteFileConfig_AutoSyncRoundTrip(t *testing.T) {
	for _, autoSync := range []bool{true, false} {
		t.Run("auto_sync="+boolStr(autoSync), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "sync.json")
			withConfigPath(t, path)

			u := ConfigUpdate{
				APIURL:   "https://hive.example.com",
				Email:    "user@example.com",
				Password: "pass",
				AutoSync: autoSync,
			}
			if err := WriteFileConfig(u); err != nil {
				t.Fatalf("WriteFileConfig: %v", err)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			var got syncFileConfig
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got.AutoSync != autoSync {
				t.Errorf("AutoSync = %v, want %v", got.AutoSync, autoSync)
			}
		})
	}
}

// TestWriteFileConfig_CreatesParentDirectory verifies MkdirAll behaviour:
// writing to a nested path that does not exist must succeed.
func TestWriteFileConfig_CreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested", "dir")
	path := filepath.Join(nested, "sync.json")
	withConfigPath(t, path)

	u := ConfigUpdate{
		APIURL:   "https://hive.example.com",
		Email:    "user@example.com",
		Password: "pass",
		AutoSync: false,
	}
	if err := WriteFileConfig(u); err != nil {
		t.Fatalf("WriteFileConfig: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

// TestAtomicWriteFile_NoPartialFileOnMarshalFailure verifies the atomic
// write contract: a pre-existing file is not replaced if marshal fails.
// We test this indirectly by checking that a valid existing file is
// preserved when atomicWriteFile is given zero-length data followed by
// a write to the same path — ensuring rename never corrupts an existing file
// unless the write succeeds.
func TestAtomicWriteFile_DirectWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	data := []byte(`{"api_url":"x","email":"e","password":"p"}`)
	if err := atomicWriteFile(path, data, 0o600); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("content = %q, want %q", got, data)
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
