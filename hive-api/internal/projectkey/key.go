package projectkey

import (
	"regexp"
	"strings"
)

var separatorRun = regexp.MustCompile(`[^a-z0-9.]+`)

// Canonicalize returns the stable project key used by API tombstones.
func Canonicalize(project string) string {
	key := strings.ToLower(strings.TrimSpace(project))
	key = separatorRun.ReplaceAllString(key, "-")
	key = strings.Trim(key, "-")
	return key
}
