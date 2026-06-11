package httpapi

import (
	"context"
	"errors"
)

// Error sentinels used by ConfigService implementations.
// Handlers map these to HTTP 400.
var (
	ErrConfigInvalidURL    = errors.New("api_url is required and must include a scheme and host")
	ErrConfigEmailRequired = errors.New("email is required")
	ErrNoStoredSecret      = errors.New("password required: no stored secret to reuse")
)

// ConfigServiceStatus is the service-level view of the current sync config.
// The raw password is NEVER present.
type ConfigServiceStatus struct {
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

// ConfigServiceUpdate carries the four fields a caller can change.
// Password may be "********" (the masked sentinel) to preserve the stored secret.
type ConfigServiceUpdate struct {
	APIURL   string
	Email    string
	Password string
	AutoSync bool
}

// ConfigServiceTestResult is the outcome of a connectivity test.
// OK is true when login succeeded. Message is human-readable and MUST NOT
// contain raw credential values.
type ConfigServiceTestResult struct {
	OK      bool
	Message string
}

// ConfigService is the interface httpapi.Server uses for config operations.
// Keeping it as an interface allows the handlers to be tested with fakes.
type ConfigService interface {
	Status(ctx context.Context) (ConfigServiceStatus, error)
	Update(ctx context.Context, req ConfigServiceUpdate) (ConfigServiceStatus, error)
	Test(ctx context.Context, req ConfigServiceUpdate) (ConfigServiceTestResult, error)
}

// ConfigStatusResponse is the wire response body for GET /governance/config/status.
type ConfigStatusResponse struct {
	Configured     bool     `json:"configured"`
	Source         string   `json:"source"`
	APIURL         string   `json:"api_url"`
	Email          string   `json:"email"`
	PasswordSet    bool     `json:"password_set"`
	PasswordMasked string   `json:"password_masked"`
	AutoSync       bool     `json:"auto_sync"`
	EnvActive      bool     `json:"env_active"`
	RestartHint    string   `json:"restart_hint,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
}

// ConfigUpdateRequest is the request body for POST /governance/config.
type ConfigUpdateRequest struct {
	APIURL   string `json:"api_url"`
	Email    string `json:"email"`
	Password string `json:"password"`
	AutoSync bool   `json:"auto_sync"`
}

// ConfigUpdateResponse wraps a refreshed ConfigStatusResponse and includes
// RestartRequired=true (always true after a successful save).
type ConfigUpdateResponse struct {
	ConfigStatusResponse
	RestartRequired bool `json:"restart_required"`
}

// ConfigTestRequest is the request body for POST /governance/config/test.
type ConfigTestRequest struct {
	APIURL   string `json:"api_url"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// ConfigTestResult is the response body for POST /governance/config/test.
// HTTP status is always 200; OK signals connectivity success.
type ConfigTestResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// statusToResponse converts a ConfigServiceStatus to a ConfigStatusResponse.
func statusToResponse(s ConfigServiceStatus) ConfigStatusResponse {
	return ConfigStatusResponse{
		Configured:     s.Configured,
		Source:         s.Source,
		APIURL:         s.APIURL,
		Email:          s.Email,
		PasswordSet:    s.PasswordSet,
		PasswordMasked: s.PasswordMasked,
		AutoSync:       s.AutoSync,
		EnvActive:      s.EnvActive,
		RestartHint:    s.RestartHint,
		Warnings:       s.Warnings,
	}
}
