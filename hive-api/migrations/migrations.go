// Package migrations expone el SQL de migración inicial embebido en el binario.
//
// Usamos go:embed para que el SQL viaje DENTRO del binario compilado.
// Esto significa que el servidor no necesita archivos externos para arrancar —
// basta con el binario. Es el enfoque 12-factor: self-contained deployments.
//
// ¿Por qué un paquete separado en lugar de embeber desde main.go?
// El directorio cmd/server/ está dos niveles por encima de migrations/,
// y go:embed no permite rutas con ".." — solo permite rutas relativas
// dentro del paquete o sus subdirectorios.
// Definiendo el embed aquí (en el paquete migrations/), la ruta "001_initial.sql"
// es válida y directa.
package migrations

import _ "embed"

// InitialSQL es el contenido del script SQL de migración inicial.
// Se incrusta en el binario en tiempo de compilación.
//
//go:embed 001_initial.sql
var InitialSQL string
