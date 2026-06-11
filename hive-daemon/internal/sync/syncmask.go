package sync

import "errors"

// MaskedSecret is the exact sentinel returned to clients in place of any stored
// password. Clients echo this value back when the user has not re-typed the
// password, and the daemon's resolvePassword reuses the stored secret instead
// of overwriting it with the sentinel.
const MaskedSecret = "********"

// Error sentinels returned by resolvePassword and the ConfigService layer.
var (
	ErrNoStoredSecret    = errors.New("password required: no stored secret to reuse")
	ErrConfigInvalidURL  = errors.New("api_url is required and must include a scheme and host")
	ErrConfigEmailRequired = errors.New("email is required")
)

// maskPassword returns MaskedSecret for any non-empty password, and "" when
// the password is empty. The raw password is never returned.
func maskPassword(raw string) string {
	if raw == "" {
		return ""
	}
	return MaskedSecret
}

// resolvePassword returns the effective password for a config update or test:
//   - If incoming != MaskedSecret, the user typed a new value — return incoming.
//   - If incoming == MaskedSecret, the user did not change the password — reuse
//     the stored secret. Returns ErrNoStoredSecret when there is nothing to reuse.
func resolvePassword(incoming string, stored *Config) (string, error) {
	if incoming != MaskedSecret {
		return incoming, nil
	}
	if stored == nil || stored.Password == "" {
		return "", ErrNoStoredSecret
	}
	return stored.Password, nil
}
