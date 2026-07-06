package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Config contiene las credenciales para conectar con hive-api.
// Se carga desde variables de entorno o desde ~/.jarvis/sync.json.
// Nunca se hardcodean en código.
type Config struct {
	APIURL   string // HIVE_API_URL   e.g. "https://hivemem.dev"
	Email    string // HIVE_API_EMAIL
	Password string // HIVE_API_PASSWORD
	AutoSync bool   // Enable automatic background sync after each mem_save (default: false)
	DaemonID string // Optional signed daemon identity for Hive API ACK authorization
}

type SyncConfigStatus struct {
	Configured bool     `json:"configured"`
	Source     string   `json:"source"`
	AutoSync   bool     `json:"auto_sync"`
	Warnings   []string `json:"warnings,omitempty"`
	// EnvActive records whether environment variables were the active runtime
	// source at load time. Retained separately so callers that overwrite Source
	// (e.g. Update setting Source to ConfigSourceFile after a write) can still
	// surface the env-override state in the API response.
	EnvActive bool `json:"env_active,omitempty"`
}

const (
	ConfigSourceEnv  = "env"
	ConfigSourceFile = "file"
	ConfigSourceNone = "none"
)

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
	AutoSync bool   `json:"auto_sync"` // Optional: enable automatic background sync after each mem_save (default: false)
	DaemonID string `json:"daemon_id,omitempty"`
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
	autoSyncStr := os.Getenv("HIVE_AUTO_SYNC")

	// Ninguna configurada → sync desactivado desde este origen
	if url == "" && email == "" && password == "" {
		return nil, false, nil
	}

	// Colectar los que faltan (auto_sync es opcional, no se incluye)
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

	// Parse HIVE_AUTO_SYNC: "true" or "1" enables auto-sync
	autoSync := autoSyncStr == "true" || autoSyncStr == "1"

	return &Config{APIURL: url, Email: email, Password: password, AutoSync: autoSync, DaemonID: os.Getenv("HIVE_DAEMON_ID")}, true, nil
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

	// Verificar permisos del archivo en Unix: deben ser exactamente 0600.
	// Windows no tiene el mismo modelo de permisos, se omite el check.
	if runtime.GOOS != "windows" {
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

	return &Config{
		APIURL:   fc.APIURL,
		Email:    fc.Email,
		Password: fc.Password,
		AutoSync: fc.AutoSync, // Defaults to false if not present in JSON
		DaemonID: fc.DaemonID,
	}, true, nil
}

// Load carga la configuración desde variables de entorno o desde ~/.jarvis/sync.json.
// El orden de precedencia es: env vars completas > archivo de configuración.
// Devuelve nil si no está configurado (sync desactivado).
// Devuelve error si no hay una configuración completa utilizable.
func Load() (*Config, error) {
	cfg, _, err := LoadWithStatus()
	return cfg, err
}

func LoadWithStatus() (*Config, SyncConfigStatus, error) {
	if cfg, ok, err := loadFromEnv(); ok || err != nil {
		if err == nil {
			return cfg, statusForConfig(ConfigSourceEnv, cfg, nil), nil
		}

		cfg, fileOK, fileErr := loadFromFile()
		if fileErr == nil && fileOK {
			warning := fmt.Sprintf("incomplete hive sync env ignored because file config is available: %v", err)
			return cfg, statusForConfig(ConfigSourceFile, cfg, []string{warning}), nil
		}
		if fileErr != nil {
			return nil, SyncConfigStatus{Configured: false, Source: ConfigSourceFile}, fmt.Errorf("%w; file config error: %v", err, fileErr)
		}
		return nil, SyncConfigStatus{Configured: false, Source: ConfigSourceNone}, err
	}

	if cfg, ok, err := loadFromFile(); ok || err != nil {
		if err != nil {
			return nil, SyncConfigStatus{Configured: false, Source: ConfigSourceFile}, err
		}
		return cfg, statusForConfig(ConfigSourceFile, cfg, nil), nil
	}

	return nil, SyncConfigStatus{Configured: false, Source: ConfigSourceNone, AutoSync: false}, nil
}

func statusForConfig(source string, cfg *Config, warnings []string) SyncConfigStatus {
	status := SyncConfigStatus{
		Configured: cfg != nil,
		Source:     source,
		EnvActive:  source == ConfigSourceEnv,
		Warnings:   warnings,
	}
	if cfg != nil {
		status.AutoSync = cfg.AutoSync
	}
	return status
}
