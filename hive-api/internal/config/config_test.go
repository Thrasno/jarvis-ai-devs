package config_test

// Nota sobre el nombre del paquete: usamos "config_test" en lugar de "config".
// Esto se llama "black-box testing" — el test accede al paquete config como
// lo haría cualquier código externo (con config.Load()), no desde dentro.
// Es la forma más realista de testear: si funciona desde fuera, funciona.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoad_ValidConfig verifica el camino feliz: todas las variables presentes.
//
// t.Setenv() es muy útil en Go: establece la variable de entorno para el test
// y la restaura automáticamente al terminar. No necesitas limpiar manualmente.
func TestLoad_ValidConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/hive")
	t.Setenv("JWT_SECRET", "esta-clave-tiene-mas-de-treinta-y-dos-caracteres")
	t.Setenv("PORT", "9090")
	t.Setenv("GIN_MODE", "release")

	cfg, err := config.Load()

	// require.NoError detiene el test inmediatamente si hay error.
	// Usar require (no assert) cuando los checks siguientes dependen de este.
	require.NoError(t, err)

	assert.Equal(t, "postgres://user:pass@localhost:5432/hive", cfg.DatabaseURL)
	assert.Equal(t, "esta-clave-tiene-mas-de-treinta-y-dos-caracteres", cfg.JWTSecret)
	assert.Equal(t, "9090", cfg.Port)
	assert.Equal(t, "release", cfg.GinMode)
	assert.False(t, cfg.ProjectBlockAdminEnabled)
}

func TestLoad_ProjectBlockAdminFeatureFlag(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/hive")
	t.Setenv("JWT_SECRET", "esta-clave-tiene-mas-de-treinta-y-dos-caracteres")
	t.Setenv("JARVIS_ENABLE_PROJECT_BLOCK_ADMIN", "true")

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.True(t, cfg.ProjectBlockAdminEnabled)
}

// TestLoad_DefaultValues verifica que PORT y GIN_MODE tienen valores por defecto
// cuando no están definidos en el entorno.
func TestLoad_DefaultValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/hive")
	t.Setenv("JWT_SECRET", "esta-clave-tiene-mas-de-treinta-y-dos-caracteres")
	// Deliberadamente NO seteamos PORT ni GIN_MODE

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, "release", cfg.GinMode)
	assert.Empty(t, cfg.DashboardAssetsDir)
}

func TestLoad_DashboardAssetsDir(t *testing.T) {
	dashboardDir := filepath.Join(t.TempDir(), "dashboard-dist")
	require.NoError(t, os.Mkdir(dashboardDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dashboardDir, "index.html"), []byte("<html>Hive Dashboard</html>"), 0o644))
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/hive")
	t.Setenv("JWT_SECRET", "esta-clave-tiene-mas-de-treinta-y-dos-caracteres")
	t.Setenv("DASHBOARD_ASSETS_DIR", dashboardDir)

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, dashboardDir, cfg.DashboardAssetsDir)
}

func TestLoad_DashboardAssetsDirValidation(t *testing.T) {
	baseDir := t.TempDir()
	validIndex := filepath.Join(baseDir, "valid-index.html")
	require.NoError(t, os.WriteFile(validIndex, []byte("<html>Hive Dashboard</html>"), 0o644))

	validDir := filepath.Join(baseDir, "valid")
	require.NoError(t, os.Mkdir(validDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(validDir, "index.html"), []byte("<html>Hive Dashboard</html>"), 0o644))

	missingIndexDir := filepath.Join(baseDir, "missing-index")
	require.NoError(t, os.Mkdir(missingIndexDir, 0o755))

	indexDirectoryDir := filepath.Join(baseDir, "index-directory")
	require.NoError(t, os.Mkdir(indexDirectoryDir, 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(indexDirectoryDir, "index.html"), 0o755))

	symlinkIndexDir := filepath.Join(baseDir, "symlink-index")
	require.NoError(t, os.Mkdir(symlinkIndexDir, 0o755))
	require.NoError(t, os.Symlink(validIndex, filepath.Join(symlinkIndexDir, "index.html")))

	unreadableIndexDir := filepath.Join(baseDir, "unreadable-index")
	require.NoError(t, os.Mkdir(unreadableIndexDir, 0o755))
	unreadableIndexPath := filepath.Join(unreadableIndexDir, "index.html")
	require.NoError(t, os.WriteFile(unreadableIndexPath, []byte("<html>Hive Dashboard</html>"), 0o644))
	require.NoError(t, os.Chmod(unreadableIndexPath, 0o000))
	t.Cleanup(func() {
		_ = os.Chmod(unreadableIndexPath, 0o644)
	})

	tests := []struct {
		name string
		path string
	}{
		{name: "missing directory", path: filepath.Join(baseDir, "missing")},
		{name: "path is file", path: validIndex},
		{name: "missing index", path: missingIndexDir},
		{name: "index is directory", path: indexDirectoryDir},
		{name: "index is symlink", path: symlinkIndexDir},
		{name: "index is unreadable", path: unreadableIndexDir},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/hive")
			t.Setenv("JWT_SECRET", "esta-clave-tiene-mas-de-treinta-y-dos-caracteres")
			t.Setenv("DASHBOARD_ASSETS_DIR", tt.path)

			_, err := config.Load()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "DASHBOARD_ASSETS_DIR")
		})
	}
}

// TestLoad_MissingDatabaseURL verifica que Load() falla si falta DATABASE_URL.
//
// Nota el patrón "t.Setenv + vacío": en algunos sistemas la variable puede
// existir de sesiones anteriores. Forzamos que esté vacía.
func TestLoad_MissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "") // forzamos que esté vacía
	t.Setenv("JWT_SECRET", "esta-clave-tiene-mas-de-treinta-y-dos-caracteres")

	_, err := config.Load()

	// assert.Error verifica que err NO es nil (que hay un error).
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL")
}

// TestLoad_MissingJWTSecret verifica que Load() falla si falta JWT_SECRET.
func TestLoad_MissingJWTSecret(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/hive")
	t.Setenv("JWT_SECRET", "")

	_, err := config.Load()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "JWT_SECRET")
}

// TestLoad_JWTSecretTooShort verifica que un JWT_SECRET menor de 32 bytes
// es rechazado. Esta es una validación de seguridad crítica — un secreto
// corto es fácil de adivinar por fuerza bruta.
func TestLoad_JWTSecretTooShort(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/hive")
	t.Setenv("JWT_SECRET", "corto") // menos de 32 bytes

	_, err := config.Load()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "32")
}

// TestLoad_JWTSecretExactly32Bytes verifica que 32 bytes es válido (el mínimo).
func TestLoad_JWTSecretExactly32Bytes(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/hive")
	t.Setenv("JWT_SECRET", "12345678901234567890123456789012") // exactamente 32 chars

	_, err := config.Load()

	assert.NoError(t, err)
}
