package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeAuthServer returns a test HTTP server that responds to POST /auth/login.
// If ok is true it returns 200 with a dummy token, otherwise 401.
func fakeAuthServer(t *testing.T, ok bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/login" && r.Method == http.MethodPost {
			if ok {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"token":      "dummy-token",
					"expires_at": "2099-01-01T00:00:00Z",
				})
			} else {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte("invalid credentials"))
			}
			return
		}
		http.NotFound(w, r)
	}))
}

// writeConfigFile writes a minimal valid config JSON to dir/sync.json at
// mode 0600 and returns the path.
func writeConfigFile(t *testing.T, dir string, fc syncFileConfig) string {
	t.Helper()
	path := filepath.Join(dir, "sync.json")
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// TestService_Status_FileSource verifies that Status returns a masked password
// and EnvActive=false when the config comes from a file.
func TestService_Status_FileSource(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	path := writeConfigFile(t, dir, syncFileConfig{
		APIURL:   "https://hive.example.com",
		Email:    "user@example.com",
		Password: "supersecret",
		AutoSync: true,
	})
	withConfigPath(t, path)

	svc := NewService()
	resp, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if !resp.Configured {
		t.Error("Configured = false, want true")
	}
	if resp.EnvActive {
		t.Error("EnvActive = true, want false for file source")
	}
	if resp.PasswordMasked != MaskedSecret {
		t.Errorf("PasswordMasked = %q, want %q", resp.PasswordMasked, MaskedSecret)
	}
	if !resp.PasswordSet {
		t.Error("PasswordSet = false, want true")
	}
	// Raw password must never appear in response
	if strings.Contains(resp.PasswordMasked, "supersecret") {
		t.Error("raw password leaked into PasswordMasked")
	}
}

// TestService_Status_EnvSource verifies that Status returns EnvActive=true
// when the config comes from environment variables.
func TestService_Status_EnvSource(t *testing.T) {
	t.Setenv("HIVE_API_URL", "https://env.example.com")
	t.Setenv("HIVE_API_EMAIL", "env@example.com")
	t.Setenv("HIVE_API_PASSWORD", "envpass")
	// Point config file to non-existent path so only env is available.
	withConfigPath(t, filepath.Join(t.TempDir(), "sync.json"))

	svc := NewService()
	resp, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if !resp.Configured {
		t.Error("Configured = false, want true")
	}
	if !resp.EnvActive {
		t.Error("EnvActive = false, want true for env source")
	}
	if resp.PasswordMasked != MaskedSecret {
		t.Errorf("PasswordMasked = %q, want %q", resp.PasswordMasked, MaskedSecret)
	}
}

// TestService_Status_Unconfigured verifies that Status returns Configured=false
// and empty PasswordMasked when no config is present.
func TestService_Status_Unconfigured(t *testing.T) {
	clearEnv(t)
	withConfigPath(t, filepath.Join(t.TempDir(), "sync.json"))

	svc := NewService()
	resp, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if resp.Configured {
		t.Error("Configured = true, want false")
	}
	if resp.PasswordSet {
		t.Error("PasswordSet = true, want false")
	}
	if resp.PasswordMasked != "" {
		t.Errorf("PasswordMasked = %q, want empty", resp.PasswordMasked)
	}
}

// TestService_Update_NewPassword verifies that Update with a new password
// calls WriteFileConfig with the real password (not the sentinel).
func TestService_Update_NewPassword(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	path := writeConfigFile(t, dir, syncFileConfig{
		APIURL:   "https://hive.example.com",
		Email:    "user@example.com",
		Password: "oldpassword",
		AutoSync: false,
	})
	withConfigPath(t, path)

	svc := NewService()
	req := ConfigUpdate{
		APIURL:   "https://hive.example.com",
		Email:    "user@example.com",
		Password: "newpassword",
		AutoSync: true,
	}
	resp, err := svc.Update(context.Background(), req)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if resp.PasswordMasked != MaskedSecret {
		t.Errorf("PasswordMasked = %q, want sentinel", resp.PasswordMasked)
	}
	if resp.RestartHint == "" {
		t.Error("RestartHint is empty, want non-empty restart message")
	}

	// Verify the file was written with the real password.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var fc syncFileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if fc.Password != "newpassword" {
		t.Errorf("written password = %q, want newpassword", fc.Password)
	}
	if !fc.AutoSync {
		t.Error("AutoSync not persisted as true")
	}
}

// TestService_Update_Sentinel verifies that Update with the masked sentinel
// preserves the stored password and does not overwrite it.
func TestService_Update_Sentinel(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	path := writeConfigFile(t, dir, syncFileConfig{
		APIURL:   "https://hive.example.com",
		Email:    "user@example.com",
		Password: "storedSecret",
		AutoSync: false,
	})
	withConfigPath(t, path)

	svc := NewService()
	req := ConfigUpdate{
		APIURL:   "https://hive.example.com",
		Email:    "user@example.com",
		Password: MaskedSecret, // sentinel — do not overwrite
		AutoSync: false,
	}
	_, err := svc.Update(context.Background(), req)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Verify the stored password was preserved.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var fc syncFileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if fc.Password != "storedSecret" {
		t.Errorf("written password = %q, want storedSecret (sentinel must not overwrite)", fc.Password)
	}
}

// TestService_Update_EnvActive verifies that Update returns RestartHint
// mentioning env vars when config source is env.
func TestService_Update_EnvActive(t *testing.T) {
	t.Setenv("HIVE_API_URL", "https://env.example.com")
	t.Setenv("HIVE_API_EMAIL", "env@example.com")
	t.Setenv("HIVE_API_PASSWORD", "envpass")
	dir := t.TempDir()
	withConfigPath(t, filepath.Join(dir, "sync.json"))

	svc := NewService()
	req := ConfigUpdate{
		APIURL:   "https://env.example.com",
		Email:    "env@example.com",
		Password: "newpass",
		AutoSync: false,
	}
	resp, err := svc.Update(context.Background(), req)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if !resp.EnvActive {
		t.Error("EnvActive = false, want true")
	}
	if !strings.Contains(resp.RestartHint, "env") {
		t.Errorf("RestartHint = %q, want hint mentioning env vars", resp.RestartHint)
	}
}

// TestService_Update_InvalidURL verifies that Update returns ErrConfigInvalidURL
// for a bad URL.
func TestService_Update_InvalidURL(t *testing.T) {
	clearEnv(t)
	withConfigPath(t, filepath.Join(t.TempDir(), "sync.json"))

	svc := NewService()
	req := ConfigUpdate{
		APIURL:   "not-a-url",
		Email:    "user@example.com",
		Password: "pass",
		AutoSync: false,
	}
	_, err := svc.Update(context.Background(), req)
	if err == nil {
		t.Fatal("Update: expected error for invalid URL, got nil")
	}
}

// TestService_Update_EmptyEmail verifies that Update returns ErrConfigEmailRequired
// for an empty email.
func TestService_Update_EmptyEmail(t *testing.T) {
	clearEnv(t)
	withConfigPath(t, filepath.Join(t.TempDir(), "sync.json"))

	svc := NewService()
	req := ConfigUpdate{
		APIURL:   "https://hive.example.com",
		Email:    "",
		Password: "pass",
		AutoSync: false,
	}
	_, err := svc.Update(context.Background(), req)
	if err == nil {
		t.Fatal("Update: expected error for empty email, got nil")
	}
}

// TestService_Test_Success verifies that Test returns ConfigTestResult{OK: true}
// when the fake server responds with 200.
func TestService_Test_Success(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	authSrv := fakeAuthServer(t, true)
	defer authSrv.Close()

	path := writeConfigFile(t, dir, syncFileConfig{
		APIURL:   authSrv.URL,
		Email:    "user@example.com",
		Password: "storedpass",
		AutoSync: false,
	})
	withConfigPath(t, path)

	svc := NewService()
	req := ConfigUpdate{
		APIURL:   authSrv.URL,
		Email:    "user@example.com",
		Password: "realpass",
		AutoSync: false,
	}
	result, err := svc.Test(context.Background(), req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !result.OK {
		t.Errorf("OK = false, want true; message: %s", result.Message)
	}
	// Raw password must not appear in message.
	if strings.Contains(result.Message, "realpass") {
		t.Error("raw password leaked into test result message")
	}
}

// TestService_Test_Failure verifies that Test returns ConfigTestResult{OK: false}
// when the fake server responds with 401.
func TestService_Test_Failure(t *testing.T) {
	clearEnv(t)
	authSrv := fakeAuthServer(t, false)
	defer authSrv.Close()

	withConfigPath(t, filepath.Join(t.TempDir(), "sync.json"))

	svc := NewService()
	req := ConfigUpdate{
		APIURL:   authSrv.URL,
		Email:    "user@example.com",
		Password: "wrongpass",
		AutoSync: false,
	}
	result, err := svc.Test(context.Background(), req)
	if err != nil {
		t.Fatalf("Test: unexpected Go error (should return ConfigTestResult): %v", err)
	}
	if result.OK {
		t.Error("OK = true, want false for auth failure")
	}
	// Raw password must not appear in message.
	if strings.Contains(result.Message, "wrongpass") {
		t.Error("raw password leaked into failure message")
	}
}

// TestService_Test_SentinelReusesStoredPassword verifies that Test with the
// masked sentinel substitutes the stored password for the connectivity attempt.
func TestService_Test_SentinelReusesStoredPassword(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	authSrv := fakeAuthServer(t, true)
	defer authSrv.Close()

	path := writeConfigFile(t, dir, syncFileConfig{
		APIURL:   authSrv.URL,
		Email:    "user@example.com",
		Password: "storedpass",
		AutoSync: false,
	})
	withConfigPath(t, path)

	svc := NewService()
	req := ConfigUpdate{
		APIURL:   authSrv.URL,
		Email:    "user@example.com",
		Password: MaskedSecret, // sentinel — reuse stored
		AutoSync: false,
	}
	result, err := svc.Test(context.Background(), req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !result.OK {
		t.Errorf("OK = false, want true (stored password is valid); message: %s", result.Message)
	}
}

// TestService_Test_DoesNotWriteFile verifies that Test never modifies the
// config file.
func TestService_Test_DoesNotWriteFile(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	authSrv := fakeAuthServer(t, true)
	defer authSrv.Close()

	path := writeConfigFile(t, dir, syncFileConfig{
		APIURL:   authSrv.URL,
		Email:    "user@example.com",
		Password: "storedpass",
		AutoSync: false,
	})
	withConfigPath(t, path)

	infoBeforeTest, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat before: %v", err)
	}

	svc := NewService()
	req := ConfigUpdate{
		APIURL:   authSrv.URL,
		Email:    "user@example.com",
		Password: "newpass",
		AutoSync: true,
	}
	_, testErr := svc.Test(context.Background(), req)
	if testErr != nil {
		t.Fatalf("Test: %v", testErr)
	}

	infoAfterTest, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat after: %v", err)
	}
	if !infoBeforeTest.ModTime().Equal(infoAfterTest.ModTime()) {
		t.Error("config file was modified by Test — must not write")
	}
}
