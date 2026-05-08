package sanitize

import (
	"bytes"
	"regexp"
	"strings"
)

// labelAttrRegex extracts the optional label attribute from a SINGLE opening
// tag span already located by the scanner. Bounded input — no backtracking risk.
var labelAttrRegex = regexp.MustCompile(`(?i)\blabel\s*=\s*"([^"]*)"`)

// scan walks s once and returns the cleaned output + outermost block count.
// It is the sole owner of depth tracking and orphan handling.
func scan(s string) (clean string, count int) {
	if s == "" {
		return "", 0
	}

	var out bytes.Buffer
	i := 0
	n := len(s)

	for i < n {
		j := indexOfPrivateOpen(s, i)
		if j == -1 {
			// No more open tags — copy remainder verbatim.
			out.WriteString(s[i:])
			break
		}

		// Copy text before the open tag verbatim.
		out.WriteString(s[i:j])

		// Locate end of opening tag '>'.
		tagEnd := strings.IndexByte(s[j:], '>')
		if tagEnd == -1 {
			// No closing '>' — orphan open: consume to EOF, emit marker, done.
			label := sanitizeLabel(extractLabel(s[j:]))
			out.WriteString(marker(label))
			count++
			break
		}
		tagEnd = j + tagEnd // absolute position of '>'

		// Extract label from the opening tag span.
		label := sanitizeLabel(extractLabel(s[j : tagEnd+1]))

		// contentStart is the first byte of block content (after the open tag's '>').
		contentStart := tagEnd + 1

		// Depth tracking: find the matching close for this open.
		depth := 1
		k := contentStart
		closeAt := -1   // absolute position of byte AFTER the matching close tag
		closeBegin := -1 // absolute position where matching close tag starts

		for k < n {
			nextOpen := indexOfPrivateOpen(s, k)
			nextClose := indexOfPrivateClose(s, k)

			if nextClose == -1 {
				// No close found — orphan open.
				break
			}

			if nextOpen != -1 && nextOpen < nextClose {
				// Nested open comes before close — descend.
				depth++
				innerTagEnd := strings.IndexByte(s[nextOpen:], '>')
				if innerTagEnd == -1 {
					// Malformed inner tag — treat outer as orphan.
					closeAt = -1
					break
				}
				k = nextOpen + innerTagEnd + 1
				continue
			}

			// Close comes first (or only close exists).
			depth--
			closeTagLen := closeTagLength(s, nextClose)
			if depth == 0 {
				closeBegin = nextClose
				closeAt = nextClose + closeTagLen
				break
			}
			k = nextClose + closeTagLen
		}

		if closeAt == -1 {
			// Orphan open — consumed to EOF, emit marker.
			out.WriteString(marker(label))
			count++
			break
		}

		// Determine if block content is empty (open tag immediately followed by close tag).
		isEmpty := closeBegin == contentStart
		if !isEmpty {
			out.WriteString(marker(label))
		}
		// Empty block emits nothing but still counts.
		count++
		i = closeAt
	}

	return out.String(), count
}

// indexOfPrivateOpen returns the index of the next case-insensitive "<private"
// followed by '>' or whitespace, starting at pos. Returns -1 if not found.
func indexOfPrivateOpen(s string, pos int) int {
	const tag = "<private"
	lower := strings.ToLower(s)
	for i := pos; i <= len(s)-len(tag); i++ {
		if lower[i:i+len(tag)] == tag {
			// Must be followed by '>' or whitespace (or end of string is malformed
			// but we handle that via tagEnd == -1 check).
			if i+len(tag) >= len(s) {
				continue
			}
			next := s[i+len(tag)]
			if next == '>' || isSpace(next) {
				return i
			}
		}
	}
	return -1
}

// indexOfPrivateClose returns the index of the next case-insensitive "</private>"
// (with optional whitespace before '>'), starting at pos. Returns -1 if not found.
func indexOfPrivateClose(s string, pos int) int {
	const tag = "</private"
	lower := strings.ToLower(s)
	for i := pos; i <= len(s)-len(tag); i++ {
		if lower[i:i+len(tag)] == tag {
			// Find the closing '>'.
			rest := s[i+len(tag):]
			j := 0
			for j < len(rest) && isSpace(rest[j]) {
				j++
			}
			if j < len(rest) && rest[j] == '>' {
				return i
			}
		}
	}
	return -1
}

// closeTagLength returns the byte length of the close tag starting at pos,
// accounting for optional whitespace before '>'.
func closeTagLength(s string, pos int) int {
	const tag = "</private"
	i := pos + len(tag)
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	if i < len(s) && s[i] == '>' {
		return i - pos + 1
	}
	return len(tag) + 1 // fallback
}

// extractLabel runs labelAttrRegex on a tag span and returns the raw label value.
// Returns "" if no label attribute is present.
func extractLabel(tagSpan string) string {
	m := labelAttrRegex.FindStringSubmatch(tagSpan)
	if m == nil {
		return ""
	}
	return m[1]
}

// isSpace reports whether b is an ASCII whitespace byte.
func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
