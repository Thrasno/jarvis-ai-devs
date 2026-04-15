package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// startPostgres inicia un contenedor PostgreSQL usando testcontainers.
// Ejecuta las migraciones embebidas y devuelve un pool de conexiones + cleanup function.
//
// Este helper se usa en TODOS los tests de repositorio para garantizar aislamiento:
// cada test obtiene un PostgreSQL real, con el schema correcto, sin estado compartido.
//
// Retorna:
// - *pgxpool.Pool: pool de conexiones listo para usar
// - func(): cleanup function que detiene el contenedor y cierra el pool
func startPostgres(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()

	ctx := context.Background()

	// Crear contenedor PostgreSQL 15 con testcontainers
	container, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err, "Failed to start PostgreSQL container")

	// Obtener connection string del contenedor
	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "Failed to get connection string")

	// Crear pool de conexiones
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err, "Failed to create connection pool")

	// Ejecutar migraciones embebidas
	_, err = pool.Exec(ctx, migrations.InitialSQL)
	require.NoError(t, err, "Failed to run migrations")

	// Cleanup function: detiene el contenedor y cierra el pool
	cleanup := func() {
		pool.Close()
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}

	return pool, cleanup
}

// truncateTables limpia todas las tablas de datos entre tests.
// Mantiene el schema intacto pero elimina todos los registros.
//
// Esto es MÁS RÁPIDO que recrear el contenedor para cada test,
// y garantiza aislamiento entre tests sin state bleed.
func truncateTables(ctx context.Context, pool *pgxpool.Pool) error {
	const q = `
		TRUNCATE TABLE users, memories RESTART IDENTITY CASCADE;
	`
	_, err := pool.Exec(ctx, q)
	if err != nil {
		return fmt.Errorf("truncate tables: %w", err)
	}
	return nil
}

// TestStartPostgres verifica que el helper de testcontainer funciona correctamente.
// Este test sirve para validar que:
// 1. testcontainers-go está instalado y funciona
// 2. El container PostgreSQL arranca correctamente
// 3. Las migraciones se ejecutan sin error
// 4. Podemos obtener un pool de conexiones válido
func TestStartPostgres(t *testing.T) {
	pool, cleanup := startPostgres(t)
	defer cleanup()

	// Verificar que el pool está activo
	require.NotNil(t, pool)

	// Verificar que podemos hacer una query simple
	var result int
	err := pool.QueryRow(context.Background(), "SELECT 1").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, 1, result)
}

// TestTruncateTables verifica que el helper de truncate funciona.
func TestTruncateTables(t *testing.T) {
	pool, cleanup := startPostgres(t)
	defer cleanup()

	ctx := context.Background()

	// Insertar un usuario de prueba
	_, err := pool.Exec(ctx, `
		INSERT INTO users (username, email, password, level)
		VALUES ('testuser', 'test@example.com', 'hashedpass', 'member')
	`)
	require.NoError(t, err)

	// Verificar que existe
	var count int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Truncar
	err = truncateTables(ctx, pool)
	require.NoError(t, err)

	// Verificar que la tabla está vacía
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
