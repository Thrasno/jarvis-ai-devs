// Package config se encarga de leer y validar la configuración de la aplicación
// desde variables de entorno.
//
// En Go (y en aplicaciones 12-factor en general) la configuración NO va en
// archivos de texto que se cargan al arrancar — va en variables de entorno.
// Esto facilita el despliegue en Docker, Kubernetes o cualquier plataforma
// sin modificar el código.
//
// Las variables requeridas son: DATABASE_URL y JWT_SECRET.
// Las opcionales con defaults son: PORT (8080) y GIN_MODE (release).
package config

import (
	"fmt"
	"os"
	"strings"
)

// Config agrupa toda la configuración que necesita la aplicación.
// Es el único lugar donde se leen variables de entorno — el resto del código
// recibe un *Config y nunca llama a os.Getenv directamente.
// Esto hace el código más testable y más fácil de razonar.
type Config struct {
	// DatabaseURL es la cadena de conexión a PostgreSQL.
	// Formato: postgres://usuario:contraseña@host:puerto/nombre_db
	DatabaseURL string

	// JWTSecret es la clave para firmar y verificar tokens JWT.
	// Mínimo 32 bytes — claves más cortas son vulnerables a fuerza bruta.
	JWTSecret string

	// Port es el puerto en el que escuchará el servidor HTTP.
	// Default: "8080"
	Port string

	// GinMode controla el nivel de verbosidad del framework HTTP Gin.
	// "debug"   → logs detallados, para desarrollo
	// "release" → silencioso, para producción
	// Default: "release"
	GinMode string

	// AllowedOrigins es la lista de orígenes permitidos para CORS.
	// Se configura con CORS_ALLOWED_ORIGINS (valores separados por coma).
	// Default: "https://hive.hivemem.dev"
	AllowedOrigins []string
}

// Load lee las variables de entorno y devuelve una Config válida.
// Si alguna variable requerida falta o es inválida, devuelve un error.
//
// El llamador (main.go) es responsable de decidir qué hacer con el error —
// normalmente terminar el proceso con os.Exit(1). Separamos la validación
// de la terminación para poder testear la validación sin matar el proceso.
func Load() (*Config, error) {
	rawOrigins := getEnvWithDefault("CORS_ALLOWED_ORIGINS", "https://hive.hivemem.dev")
	var origins []string
	for _, o := range strings.Split(rawOrigins, ",") {
		if trimmed := strings.TrimSpace(o); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}

	cfg := &Config{
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		Port:           getEnvWithDefault("PORT", "8080"),
		GinMode:        getEnvWithDefault("GIN_MODE", "release"),
		AllowedOrigins: origins,
	}

	// Validamos cada campo requerido.
	// fmt.Errorf crea un error con un mensaje formateado — es el equivalente
	// a new Error(`mensaje ${variable}`) en JavaScript o
	// throw new \Exception("mensaje $variable") en PHP.
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL es requerida")
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET es requerida")
	}

	// len() en un string de Go devuelve el número de BYTES, no de caracteres.
	// Para texto ASCII (que es nuestro caso con secretos generados con openssl)
	// bytes == caracteres. Para texto con emojis o acentos, sería diferente.
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET debe tener al menos 32 bytes (tiene %d)", len(cfg.JWTSecret))
	}

	return cfg, nil
}

// getEnvWithDefault lee una variable de entorno y devuelve el valor por defecto
// si la variable no existe o está vacía.
//
// Es una función privada (minúscula) — solo accesible dentro del paquete config.
// En Go, Mayúscula = público (exportado), minúscula = privado (interno al paquete).
// Es el equivalente a public/private en PHP, pero aplicado al nombre en lugar
// de una palabra clave.
func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
