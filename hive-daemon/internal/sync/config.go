package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Config contiene las credenciales para conectar con hive-api.
// Se carga desde variables de entorno o desde ~/.jarvis/sync.json.
// Nunca se hardcodean en código.
type Config struct {
	APIURL   string // HIVE_API_URL   e.g. "https://hivemem.dev"
	Email    string // HIVE_API_EMAIL
	Password string // HIVE_API_PASSWORD
}

// configFilePath es una función variable para que los tests puedan sustituirla.
// En producción apunta a defaultConfigPath.
var configFilePath = defaultConfigPath

// defaultConfigPath devuelve ~/.jarvis/sync.json.
func defaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home dir: %w", err)
	}
	return filepath.Join(home, ".jarvis", "sync.json"), nil
}

// syncFileConfig es la estructura JSON del archivo de configuración.
type syncFileConfig struct {
	APIURL   string `json:"api_url"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// loadFromEnv intenta cargar la configuración desde variables de entorno.
//
// Retorna:
//   - (nil, false, nil)  → ninguna var presente → pasar al siguiente origen
//   - (cfg, true, nil)   → todas presentes → éxito
//   - (nil, true, err)   → parcialmente presentes → error explicativo
func loadFromEnv() (*Config, bool, error) {
	url := os.Getenv("HIVE_API_URL")
	email := os.Getenv("HIVE_API_EMAIL")
	password := os.Getenv("HIVE_API_PASSWORD")

	// Ninguna configurada → sync desactivado desde este origen
	if url == "" && email == "" && password == "" {
		return nil, false, nil
	}

	// Colectar los que faltan
	var missing []string
	if url == "" {
		missing = append(missing, "HIVE_API_URL")
	}
	if email == "" {
		missing = append(missing, "HIVE_API_EMAIL")
	}
	if password == "" {
		missing = append(missing, "HIVE_API_PASSWORD")
	}

	if len(missing) > 0 {
		return nil, true, fmt.Errorf(
			"incomplete hive sync env: set HIVE_API_URL, HIVE_API_EMAIL, HIVE_API_PASSWORD: missing %s",
			strings.Join(missing, ", "),
		)
	}

	return &Config{APIURL: url, Email: email, Password: password}, true, nil
}

// loadFromFile intenta cargar la configuración desde ~/.jarvis/sync.json.
//
// Retorna:
//   - (nil, false, nil)  → archivo no existe → pasar al siguiente origen
//   - (cfg, true, nil)   → archivo válido → éxito
//   - (nil, true, err)   → archivo existe pero hay error → error explicativo
func loadFromFile() (*Config, bool, error) {
	path, err := configFilePath()
	if err != nil {
		return nil, true, err
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, true, fmt.Errorf("stat %s: %w", path, err)
	}

	// Verificar permisos del archivo: deben ser exactamente 0600
	if info.Mode().Perm()&0o077 != 0 {
		return nil, true, fmt.Errorf(
			"insecure permissions on %s: 0%o (must be 0600); run: chmod 600 %s",
			path, info.Mode().Perm(), path,
		)
	}

	// Verificar permisos del directorio padre (warn-only, no fatal)
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err == nil && dirInfo.Mode().Perm()&^os.FileMode(0o700) != 0 {
		log.Printf("hive-sync: warning: directory %s has permissions 0%o (recommend 0700)",
			filepath.Dir(path), dirInfo.Mode().Perm())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, true, fmt.Errorf("read %s: %w", path, err)
	}

	var fc syncFileConfig
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&fc); err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", path, err)
	}

	// Verificar que no falten campos obligatorios
	var missing []string
	if fc.APIURL == "" {
		missing = append(missing, "api_url")
	}
	if fc.Email == "" {
		missing = append(missing, "email")
	}
	if fc.Password == "" {
		missing = append(missing, "password")
	}
	if len(missing) > 0 {
		return nil, true, fmt.Errorf("incomplete %s: missing %s", path, strings.Join(missing, ", "))
	}

	return &Config{APIURL: fc.APIURL, Email: fc.Email, Password: fc.Password}, true, nil
}

// Load carga la configuración desde variables de entorno o desde ~/.jarvis/sync.json.
// El orden de precedencia es: env vars > archivo de configuración.
// Devuelve nil si no está configurado (sync desactivado).
// Devuelve error si está parcialmente configurado (evita confusión).
func Load() (*Config, error) {
	if cfg, ok, err := loadFromEnv(); ok || err != nil {
		return cfg, err
	}
	if cfg, ok, err := loadFromFile(); ok || err != nil {
		return cfg, err
	}
	return nil, nil
}
