package projectkey

import "github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"

// Canonicalize returns the stable project key used by API tombstones.
func Canonicalize(project string) string {
	return projectidentity.Canonical(project).String()
}
