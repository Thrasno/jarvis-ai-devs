package model

import "github.com/golang-jwt/jwt/v5"

// Claims define el contenido (payload) de los tokens JWT que emite hive-api.
//
// Un JWT tiene campos estándar (exp, iat, sub...) y campos personalizados.
// Embebemos jwt.RegisteredClaims para obtener los estándar gratis,
// y añadimos los nuestros debajo.
//
// Cuando el middleware de autenticación valida un token, deserializa
// el payload en este struct y lo inyecta en el contexto de la request.
// Los handlers lo extraen del contexto para saber quién está llamando.
type Claims struct {
	// jwt.RegisteredClaims aporta los campos estándar del protocolo JWT:
	//   - Subject (sub): identificador del usuario (UUID)
	//   - ExpiresAt (exp): cuándo expira el token
	//   - IssuedAt (iat): cuándo se generó
	// Al estar embebido (sin nombre de campo), sus métodos y campos
	// son accesibles directamente: claims.Subject, claims.ExpiresAt, etc.
	jwt.RegisteredClaims

	// Username es el nombre de usuario, incluido en el token para que
	// los handlers puedan usarlo sin consultar la base de datos.
	Username string `json:"username"`

	// Level es el nivel de acceso. Lo incluimos en el token para que
	// el middleware admin pueda verificarlo sin tocar la DB en cada request.
	Level UserLevel `json:"level"`
}
