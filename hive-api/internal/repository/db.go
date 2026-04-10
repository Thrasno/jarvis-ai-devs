package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool crea y valida un pool de conexiones a PostgreSQL.
//
// Un "pool" es un conjunto de conexiones reutilizables. En lugar de abrir
// y cerrar una conexión por cada query (caro), el pool mantiene un grupo
// de conexiones abiertas y las presta a quien las necesite.
//
// pgxpool es la implementación de pool de pgx — la librería PostgreSQL
// más eficiente para Go. Es el equivalente a un connection pool de PDO en PHP,
// pero mucho más configurable.
//
// La función valida la conexión con Ping antes de devolver el pool.
// Si PostgreSQL no está disponible, falla aquí (en el arranque) y no
// en medio de un request.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsear DATABASE_URL: %w", err)
	}

	// Configuración del pool para un equipo pequeño (8 developers).
	// Estos valores están calibrados para no saturar PostgreSQL en un VPS pequeño.
	cfg.MaxConns = 20                      // máximo 20 conexiones simultáneas
	cfg.MinConns = 2                       // mínimo 2 siempre abiertas (warm pool)
	cfg.MaxConnLifetime = 30 * time.Minute // reciclar conexiones cada 30 minutos
	cfg.MaxConnIdleTime = 5 * time.Minute  // cerrar conexiones inactivas tras 5 min
	cfg.HealthCheckPeriod = 30 * time.Second // verificar conexiones cada 30 seg

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("crear pool de conexiones: %w", err)
	}

	// Ping verifica que realmente podemos conectar.
	// Falla rápido en el arranque en lugar de fallar en el primer request.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping a PostgreSQL fallido: %w", err)
	}

	return pool, nil
}

// RunMigrations ejecuta las migraciones SQL al arrancar el servidor.
//
// En lugar de una herramienta de migraciones compleja, para MVP 1 usamos
// el enfoque más simple: un único archivo SQL con CREATE TABLE IF NOT EXISTS.
// "IF NOT EXISTS" hace las migraciones idempotentes — si ya existen las tablas,
// no hace nada. En MVP 2 migraremos a golang-migrate para versiones incrementales.
func RunMigrations(pool *pgxpool.Pool, migrationSQL string) error {
	_, err := pool.Exec(context.Background(), migrationSQL)
	if err != nil {
		return fmt.Errorf("ejecutar migraciones: %w", err)
	}
	return nil
}
