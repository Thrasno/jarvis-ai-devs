package sanitize

import "regexp"

// labelAttrRegex extracts the optional label attribute from a SINGLE opening
// tag span already located by the scanner. Bounded input — no backtracking risk.
var labelAttrRegex = regexp.MustCompile(`(?i)\blabel\s*=\s*"([^"]*)"`)

// scan walks s once and returns the cleaned output + outermost block count.
// Stub: returns s unchanged with count 0. Replaced in T-03.
func scan(s string) (string, int) {
	return s, 0
}
