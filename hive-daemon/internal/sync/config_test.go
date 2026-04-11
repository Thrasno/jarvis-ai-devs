package sync

import (
	"os"
	"path/filepath"
	"testing"
)

// clearEnv limpia las 3 variables de entorno de sync usando t.Setenv
// (que las restaura automáticamente al finalizar el test).
func clearEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HIVE_API_URL", "")
	t.Setenv("HIVE_API_EMAIL", "")
	t.Setenv("HIVE_API_PASSWORD", "")
}

// writeFile escribe body en dir/sync.json con el modo dado y retorna el path.
func writeFile(t *testing.T, dir, body string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, "sync.json")
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	return path
}

// withConfigPath sustituye la var configFilePath por una función que retorna
// el path dado, y la restaura en t.Cleanup.
func withConfigPath(t *testing.T, path string) {
	t.Helper()
	orig := configFilePath
	configFilePath = func() (string, error) { return path, nil }
	t.Cleanup(func() { configFilePath = orig })
}

// validJSON es un archivo de configuración completo y válido.
const validJSON = `{"api_url":"https://hive.example.com","email":"user@example.com","password":"s3cr3t"}`

func TestLoad(t *testing.T) {
	cases := []struct {
		name        string
		setup       func(t *testing.T)
		wantNilCfg  bool
		wantNilErr  bool
		errContains string
		wantURL     string
	}{
		{
			name: "no env, no file",
			setup: func(t *testing.T) {
				clearEnv(t)
				// apuntar a un path que no existe
				withConfigPath(t, filepath.Join(t.TempDir(), "sync.json"))
			},
			wantNilCfg: true,
			wantNilErr: true,
		},
		{
			name: "valid file 0600, no env",
			setup: func(t *testing.T) {
				clearEnv(t)
				dir := t.TempDir()
				path := writeFile(t, dir, validJSON, 0600)
				withConfigPath(t, path)
			},
			wantNilCfg: false,
			wantNilErr: true,
			wantURL:    "https://hive.example.com",
		},
		{
			name: "valid env, no file",
			setup: func(t *testing.T) {
				t.Setenv("HIVE_API_URL", "https://env.example.com")
				t.Setenv("HIVE_API_EMAIL", "env@example.com")
				t.Setenv("HIVE_API_PASSWORD", "envpass")
				withConfigPath(t, filepath.Join(t.TempDir(), "sync.json"))
			},
			wantNilCfg: false,
			wantNilErr: true,
			wantURL:    "https://env.example.com",
		},
		{
			name: "both present — env wins",
			setup: func(t *testing.T) {
				t.Setenv("HIVE_API_URL", "https://env-wins.example.com")
				t.Setenv("HIVE_API_EMAIL", "env@example.com")
				t.Setenv("HIVE_API_PASSWORD", "envpass")
				dir := t.TempDir()
				path := writeFile(t, dir, validJSON, 0600)
				withConfigPath(t, path)
			},
			wantNilCfg: false,
			wantNilErr: true,
			wantURL:    "https://env-wins.example.com",
		},
		{
			name: "partial env (only URL)",
			setup: func(t *testing.T) {
				t.Setenv("HIVE_API_URL", "https://partial.example.com")
				t.Setenv("HIVE_API_EMAIL", "")
				t.Setenv("HIVE_API_PASSWORD", "")
				withConfigPath(t, filepath.Join(t.TempDir(), "sync.json"))
			},
			wantNilCfg:  true,
			wantNilErr:  false,
			errContains: "missing",
		},
		{
			name: "malformed JSON",
			setup: func(t *testing.T) {
				clearEnv(t)
				dir := t.TempDir()
				path := writeFile(t, dir, `{not valid json`, 0600)
				withConfigPath(t, path)
			},
			wantNilCfg:  true,
			wantNilErr:  false,
			errContains: "parse",
		},
		{
			name: "file missing password field",
			setup: func(t *testing.T) {
				clearEnv(t)
				dir := t.TempDir()
				path := writeFile(t, dir, `{"api_url":"https://x.com","email":"u@x.com"}`, 0600)
				withConfigPath(t, path)
			},
			wantNilCfg:  true,
			wantNilErr:  false,
			errContains: "missing password",
		},
		{
			name: "file perms 0644",
			setup: func(t *testing.T) {
				clearEnv(t)
				dir := t.TempDir()
				path := writeFile(t, dir, validJSON, 0644)
				withConfigPath(t, path)
			},
			wantNilCfg:  true,
			wantNilErr:  false,
			errContains: "insecure permissions",
		},
		{
			name: "file absent, no env",
			setup: func(t *testing.T) {
				clearEnv(t)
				withConfigPath(t, filepath.Join(t.TempDir(), "nonexistent.json"))
			},
			wantNilCfg: true,
			wantNilErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)

			cfg, err := Load()

			if tc.wantNilErr && err != nil {
				t.Fatalf("expected nil error, got: %v", err)
			}
			if !tc.wantNilErr && err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if tc.errContains != "" && err != nil {
				if !contains(err.Error(), tc.errContains) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.errContains)
				}
			}
			if tc.wantNilCfg && cfg != nil {
				t.Fatalf("expected nil config, got: %+v", cfg)
			}
			if !tc.wantNilCfg && cfg == nil {
				t.Fatalf("expected non-nil config, got nil")
			}
			if tc.wantURL != "" && cfg != nil && cfg.APIURL != tc.wantURL {
				t.Fatalf("expected APIURL %q, got %q", tc.wantURL, cfg.APIURL)
			}
		})
	}
}

func TestDefaultConfigPath(t *testing.T) {
	path, err := defaultConfigPath()
	if err != nil {
		t.Fatalf("defaultConfigPath() error: %v", err)
	}
	if path == "" {
		t.Fatal("defaultConfigPath() returned empty string")
	}
	// Debe terminar en .jarvis/sync.json
	if !contains(path, ".jarvis") || !contains(path, "sync.json") {
		t.Fatalf("unexpected path: %q", path)
	}
}

func TestLoadFromFileUnknownField(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	// DisallowUnknownFields debe rechazar campos desconocidos
	path := writeFile(t, dir, `{"api_url":"x","email":"e","password":"p","unknown":"bad"}`, 0600)
	withConfigPath(t, path)

	cfg, err := Load()
	if err == nil {
		t.Fatalf("expected error for unknown field, got nil (cfg=%+v)", cfg)
	}
	if !contains(err.Error(), "parse") {
		t.Fatalf("error %q does not contain 'parse'", err.Error())
	}
}

func TestLoadFromFileDirWidePerms(t *testing.T) {
	// El directorio con permisos más amplios que 0700 es un warn-only,
	// no debe retornar error.
	clearEnv(t)
	dir := t.TempDir()
	// Ampliar permisos del directorio (0755 tiene g+r,o+r)
	if err := os.Chmod(dir, 0755); err != nil {
		t.Skip("cannot chmod dir:", err)
	}
	path := writeFile(t, dir, validJSON, 0600)
	withConfigPath(t, path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected nil error (dir perm is warn-only), got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

// contains es un helper para no importar strings en los tests.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
