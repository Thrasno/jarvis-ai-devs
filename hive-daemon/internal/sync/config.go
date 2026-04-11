package sync

import (
	"errors"
	"os"
)

// Config contiene las credenciales para conectar con hive-api.
// Se carga desde variables de entorno — nunca se hardcodean en código.
type Config struct {
	APIURL   string // HIVE_API_URL   e.g. "https://hivemem.dev"
	Email    string // HIVE_API_EMAIL
	Password string // HIVE_API_PASSWORD
}

// Load carga la configuración desde variables de entorno.
// Devuelve nil si no están configuradas (sync desactivado).
// Devuelve error si están parcialmente configuradas (evita confusión).
func Load() (*Config, error) {
	url := os.Getenv("HIVE_API_URL")
	email := os.Getenv("HIVE_API_EMAIL")
	password := os.Getenv("HIVE_API_PASSWORD")

	// Si ninguna está configurada → sync desactivado (modo local puro)
	if url == "" && email == "" && password == "" {
		return nil, nil
	}

	// Si están parcialmente configuradas → error explicativo
	if url == "" {
		return nil, errors.New("HIVE_API_URL es requerido cuando HIVE_API_EMAIL/PASSWORD están configurados")
	}
	if email == "" {
		return nil, errors.New("HIVE_API_EMAIL es requerido")
	}
	if password == "" {
		return nil, errors.New("HIVE_API_PASSWORD es requerido")
	}

	return &Config{
		APIURL:   url,
		Email:    email,
		Password: password,
	}, nil
}
