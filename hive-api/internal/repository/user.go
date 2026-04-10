// Package repository define los contratos de acceso a datos y sus implementaciones.
//
// Cada repositorio tiene dos partes:
//   1. Una interfaz (el contrato) — qué operaciones están disponibles
//   2. Una implementación concreta (postgres*) — cómo se hacen esas operaciones
//
// El resto del sistema (services) solo conoce la interfaz, nunca la implementación.
// Esto permite testear los services con mocks sin necesitar una base de datos real.
package repository

import (
	"context"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
)

// UserRepository define todas las operaciones de base de datos para usuarios.
//
// Es una interfaz — solo declara qué métodos existen, no cómo están implementados.
// La implementación real (PostgreSQL) está en postgresUserRepository, más abajo.
// Los mocks para tests implementan esta misma interfaz con datos en memoria.
//
// Nota sobre context.Context: es el primer parámetro en todos los métodos.
// Propaga cancelaciones y timeouts desde el request HTTP hasta la base de datos.
// Si el cliente cancela la petición, las queries en curso se cancelan también.
type UserRepository interface {
	// Create inserta un nuevo usuario. Devuelve error si el username o email
	// ya existen (violación de UNIQUE constraint).
	Create(ctx context.Context, user *model.User) (*model.User, error)

	// GetByID busca un usuario por su UUID. Devuelve ErrNotFound si no existe.
	GetByID(ctx context.Context, id string) (*model.User, error)

	// GetByEmail busca un usuario por email. Devuelve ErrNotFound si no existe.
	// Se usa en el login para verificar credenciales.
	GetByEmail(ctx context.Context, email string) (*model.User, error)

	// GetByUsername busca un usuario por username. Devuelve ErrNotFound si no existe.
	// Se usa en los endpoints de admin (/admin/users/:username/...).
	GetByUsername(ctx context.Context, username string) (*model.User, error)

	// List devuelve todos los usuarios. Solo lo usa el admin para estadísticas.
	List(ctx context.Context) ([]*model.User, error)

	// UpdateLevel cambia el nivel de acceso de un usuario.
	UpdateLevel(ctx context.Context, id string, level model.UserLevel) error

	// CountAdmins devuelve cuántos usuarios tienen nivel admin.
	// Se usa para enforcement del límite de 3 admins máximo.
	CountAdmins(ctx context.Context) (int, error)

	// Deactivate deshabilita un usuario (is_active = false).
	// No borra el registro — preservamos el historial.
	Deactivate(ctx context.Context, id string) error
}
