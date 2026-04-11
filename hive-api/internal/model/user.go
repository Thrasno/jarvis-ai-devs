// Package model contiene las estructuras de datos del dominio.
// En Go, los "modelos" son structs simples — solo datos, sin lógica de negocio.
// La lógica vive en los "services" (capa superior).
package model

import "time"

// UserLevel representa el nivel de acceso de un usuario en el sistema.
//
// Usamos un tipo propio en lugar de un string plano para que el compilador
// nos avise si alguien intenta usar un valor no válido. Es como un enum
// en otros lenguajes.
type UserLevel string

const (
	// LevelViewer puede leer memorias pero no crear ni sincronizar.
	LevelViewer UserLevel = "viewer"

	// LevelMember puede leer, crear memorias y sincronizar. Nivel por defecto.
	LevelMember UserLevel = "member"

	// LevelAdmin tiene acceso completo, incluidos los endpoints de administración.
	LevelAdmin UserLevel = "admin"
)

// IsValid comprueba si el nivel es uno de los valores permitidos.
//
// En Go los métodos se definen FUERA del struct, pero asociados a él
// mediante un "receiver" — el "(l UserLevel)" antes del nombre de función.
// Es el equivalente a un método de instancia en PHP: $level->isValid()
func (l UserLevel) IsValid() bool {
	switch l {
	case LevelViewer, LevelMember, LevelAdmin:
		return true
	}
	return false
}

// User representa un usuario del sistema tal como existe en la base de datos.
//
// Los campos con `json:"-"` no se incluyen NUNCA en la respuesta JSON.
// Esto es crítico para Password: nunca debe salir del servidor, ni siquiera
// como hash. El guión significa "omitir siempre en JSON".
//
// Los campos con `json:"omitempty"` solo aparecen en el JSON si tienen valor.
// Por ejemplo, UpdatedAt solo se incluye si no es la fecha cero de Go.
type User struct {
	// ID es un UUID generado por PostgreSQL al insertar. Lo manejamos como
	// string en Go para simplificar — pgx lo convierte automáticamente.
	ID string `json:"id"`

	Username string `json:"username"`
	Email    string `json:"email"`

	// Password contiene el hash bcrypt. El json:"-" garantiza que JAMÁS
	// aparezca en ninguna respuesta de la API, aunque alguien lo intente.
	Password string `json:"-"`

	Level    UserLevel `json:"level"`
	IsActive bool      `json:"is_active"`

	// time.Time es el tipo de Go para fechas/horas. Cuando se serializa a JSON,
	// Go lo convierte automáticamente a formato RFC3339 (ej: "2026-04-10T20:00:00Z"),
	// que es el estándar en APIs REST.
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
