// Package sanitize strips dev-marked private content from user-supplied text
// at the persistence boundary. It is intentionally minimal: no pattern detection,
// no entropy heuristics — only explicit <private> tags.
package sanitize

import "regexp"

// labelSafeRegex matches a single character that is NOT valid in a sanitized label.
// Uses single-char match (no '+') so each invalid char maps to one '-', preserving count.
var labelSafeRegex = regexp.MustCompile(`[^a-z0-9-]`)

// Result is the outcome of a Strip call. Count is the number of OUTERMOST
// private blocks replaced. Nested inner blocks contribute zero to Count.
type Result struct {
	Clean string
	Count int
}

// Strip removes <private> blocks from s, replacing each OUTERMOST block
// (including any nested children) with "[REDACTED]" or "[REDACTED:label]"
// when a label="..." attribute is present on the OUTER opening tag.
// Pure function; safe for concurrent use; never returns an error.
func Strip(s string) Result {
	clean, count := scan(s)
	return Result{Clean: clean, Count: count}
}

// StripFields applies Strip to multiple named fields and aggregates the count.
// Convenience used at handler call sites that strip title + content together.
func StripFields(fields map[string]string) (cleaned map[string]string, total int) {
	cleaned = make(map[string]string, len(fields))
	for k, v := range fields {
		r := Strip(v)
		cleaned[k] = r.Clean
		total += r.Count
	}
	return cleaned, total
}

// sanitizeLabel normalizes an arbitrary label capture into a safe slug:
//   - lowercased
//   - chars outside [a-z0-9-] replaced with '-'
//   - truncated to 32 chars
//   - empty result yields "" (caller emits bare [REDACTED])
func sanitizeLabel(raw string) string {
	if raw == "" {
		return ""
	}
	lower := toLower(raw)
	slugged := labelSafeRegex.ReplaceAllString(lower, "-")
	if len(slugged) > 32 {
		slugged = slugged[:32]
	}
	return slugged
}

// toLower returns s lowercased using only ASCII rules (safe for label slugging).
func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// marker builds the replacement string for a stripped block.
// If label is empty, returns "[REDACTED]"; otherwise "[REDACTED:label]".
func marker(label string) string {
	if label == "" {
		return "[REDACTED]"
	}
	return "[REDACTED:" + label + "]"
}
