package sync

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// ConfigStatus is the service-level representation of the current sync config.
// It is produced by the sync package and consumed by httpapi handlers which
// map it to httpapi.ConfigStatusResponse for the wire.
type ConfigStatus struct {
	Configured     bool
	Source         string
	APIURL         string
	Email          string
	PasswordSet    bool
	PasswordMasked string
	AutoSync       bool
	EnvActive      bool
	RestartHint    string
	Warnings       []string
}

// ConfigTestResult is the outcome of a Test call.
// OK is true when the connectivity check succeeded.
// Message is human-readable and MUST NOT contain raw credential values.
type ConfigTestResult struct {
	OK      bool
	Message string
}

// ConfigServicer is the interface the httpapi.Server depends on.
// Using a distinct name avoids collision with Go's built-in naming conventions.
type ConfigServicer interface {
	Status(ctx context.Context) (ConfigStatus, error)
	Update(ctx context.Context, req ConfigUpdate) (ConfigStatus, error)
	Test(ctx context.Context, req ConfigUpdate) (ConfigTestResult, error)
}

// Service is the concrete implementation of ConfigServicer.
// It is constructed with NewService and wired into httpapi.Server in main.go.
type Service struct{}

// NewService constructs a Service.
func NewService() *Service {
	return &Service{}
}

// Status loads the current sync config and returns a masked representation.
func (s *Service) Status(_ context.Context) (ConfigStatus, error) {
	cfg, status, err := LoadWithStatus()
	if err != nil {
		return ConfigStatus{}, fmt.Errorf("load config: %w", err)
	}
	return buildConfigStatus(cfg, status, ""), nil
}

// Update validates the request, resolves the effective password (sentinel-aware),
// writes the config atomically, and returns the updated status.
func (s *Service) Update(_ context.Context, req ConfigUpdate) (ConfigStatus, error) {
	if err := validateConfigUpdate(req); err != nil {
		return ConfigStatus{}, err
	}

	// Load current config to resolve the sentinel.
	cfg, status, err := LoadWithStatus()
	if err != nil {
		return ConfigStatus{}, fmt.Errorf("load config: %w", err)
	}

	effectivePassword, err := resolvePassword(req.Password, cfg)
	if err != nil {
		return ConfigStatus{}, err
	}

	toWrite := ConfigUpdate{
		APIURL:   req.APIURL,
		Email:    req.Email,
		Password: effectivePassword,
		AutoSync: req.AutoSync,
	}
	if err := WriteFileConfig(toWrite); err != nil {
		return ConfigStatus{}, fmt.Errorf("write config: %w", err)
	}

	hint := restartHint(status.Source)
	return buildConfigStatus(cfg, status, hint), nil
}

// Test validates the request, resolves the effective password, constructs a
// transient sync client, and attempts to log in. It never writes any file.
// A connectivity failure is reported as ConfigTestResult{OK: false} — not a Go error.
func (s *Service) Test(_ context.Context, req ConfigUpdate) (ConfigTestResult, error) {
	if err := validateConfigUpdate(req); err != nil {
		// Validation errors are returned as Go errors so the handler can map to 400.
		return ConfigTestResult{}, err
	}

	// Load current config to resolve the sentinel (read-only; does not write).
	cfg, _, err := LoadWithStatus()
	if err != nil {
		return ConfigTestResult{}, fmt.Errorf("load config: %w", err)
	}

	effectivePassword, err := resolvePassword(req.Password, cfg)
	if err != nil {
		return ConfigTestResult{}, err
	}

	// Construct a transient client with the test credentials.
	testCfg := &Config{
		APIURL:   req.APIURL,
		Email:    req.Email,
		Password: effectivePassword,
	}
	c := newClient(testCfg)

	if loginErr := c.Login(context.Background()); loginErr != nil {
		// Sanitise: never expose the effective password in the message.
		msg := loginErr.Error()
		msg = strings.ReplaceAll(msg, effectivePassword, "[REDACTED]")
		return ConfigTestResult{OK: false, Message: "Connection failed: " + msg}, nil
	}
	return ConfigTestResult{OK: true, Message: "Connection succeeded"}, nil
}

// validateConfigUpdate checks that api_url and email pass basic requirements.
func validateConfigUpdate(req ConfigUpdate) error {
	trimURL := strings.TrimSpace(req.APIURL)
	if trimURL == "" {
		return ErrConfigInvalidURL
	}
	u, err := url.Parse(trimURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ErrConfigInvalidURL
	}
	if strings.TrimSpace(req.Email) == "" {
		return ErrConfigEmailRequired
	}
	return nil
}

// buildConfigStatus constructs a ConfigStatus from loaded config + status.
// hint is an optional restart message set by callers that mutate the config.
func buildConfigStatus(cfg *Config, status SyncConfigStatus, hint string) ConfigStatus {
	var cs ConfigStatus
	cs.Configured = status.Configured
	cs.Source = status.Source
	cs.AutoSync = status.AutoSync
	cs.Warnings = status.Warnings
	cs.EnvActive = status.Source == ConfigSourceEnv
	cs.RestartHint = hint

	if cfg != nil {
		cs.APIURL = cfg.APIURL
		cs.Email = cfg.Email
		cs.PasswordSet = cfg.Password != ""
		cs.PasswordMasked = maskPassword(cfg.Password)
	}
	return cs
}

// restartHint returns the appropriate restart message based on config source.
func restartHint(source string) string {
	if source == ConfigSourceEnv {
		return "Saved to ~/.jarvis/sync.json. " +
			"env variable overrides are active at runtime; " +
			"restart hive-daemon after unsetting HIVE_API_* for the file values to take effect."
	}
	return "Saved. Restart hive-daemon for the new configuration to take effect."
}
