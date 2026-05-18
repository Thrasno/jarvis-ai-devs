package tui

import _ "embed"

//go:embed assets/nexus-logo-braille-64col.txt
var cockpitLogo string

// CockpitLogo returns the embedded text-only Nexus logo used by the cockpit.
func CockpitLogo() string {
	return cockpitLogo
}
