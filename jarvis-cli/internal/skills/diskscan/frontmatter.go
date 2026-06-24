// Package diskscan provides on-disk skill discovery by walking skill directories
// and parsing SKILL.md frontmatter.
package diskscan

import (
	"strings"
)

// FrontmatterResult holds the parsed frontmatter fields from a SKILL.md file.
type FrontmatterResult struct {
	Name        string
	DisplayName string
	Description string
	Trigger     string
	Scope       string
}

// FrontmatterWarning describes a problem found while parsing frontmatter.
type FrontmatterWarning struct {
	Code string
	Path string
}

// ParseFrontmatter parses the YAML frontmatter block from content and returns
// the extracted fields. If name is missing, a missing-name warning is returned.
// If trigger is missing (but name is present), a missing-trigger warning is returned.
// Missing scope is allowed and returns no warning.
func ParseFrontmatter(content []byte, path string) (FrontmatterResult, *FrontmatterWarning) {
	block, _ := extractFrontmatterBlock(content)

	result := parseFrontmatterFields(block)

	if result.Name == "" {
		return result, &FrontmatterWarning{Code: "missing-name", Path: path}
	}
	if result.Trigger == "" {
		return result, &FrontmatterWarning{Code: "missing-trigger", Path: path}
	}
	return result, nil
}

// extractFrontmatterBlock extracts the raw YAML content between the opening and
// closing --- delimiters. Returns (content, true) when a valid block is found,
// or (nil, false) when absent.
func extractFrontmatterBlock(content []byte) ([]byte, bool) {
	s := string(content)

	// Strip a leading UTF-8 BOM if present.
	s = strings.TrimPrefix(s, "\xEF\xBB\xBF")

	// Must start with ---
	if !strings.HasPrefix(s, "---") {
		return nil, false
	}
	// Find the newline after the opening ---
	firstNewline := strings.IndexByte(s, '\n')
	if firstNewline < 0 {
		return nil, false
	}
	rest := s[firstNewline+1:]

	// Find the closing ---
	closingIdx := -1
	for {
		idx := strings.Index(rest, "---")
		if idx < 0 {
			break
		}
		// The --- must be at the beginning of a line
		if idx == 0 || rest[idx-1] == '\n' {
			// Ensure the rest of the line after --- is empty or whitespace
			afterDelim := rest[idx+3:]
			lineEnd := strings.IndexByte(afterDelim, '\n')
			var lineRemainder string
			if lineEnd < 0 {
				lineRemainder = afterDelim
			} else {
				lineRemainder = afterDelim[:lineEnd]
			}
			if strings.TrimSpace(lineRemainder) == "" {
				closingIdx = idx
				break
			}
		}
		rest = rest[idx+3:]
	}

	if closingIdx < 0 {
		return nil, false
	}

	block := rest[:closingIdx]
	return []byte(block), true
}

// parseFrontmatterFields does a minimal line-by-line parse of YAML frontmatter.
// Only top-level scalar keys are extracted; nested structures are ignored.
//
// Trigger resolution order (gentle v1.42.0 spec):
//  1. Standalone "Trigger:" (or "trigger:") key — explicit override, highest priority.
//  2. "Trigger:" text embedded inside the description field value — handles both
//     single-line quoted values and YAML folded/block scalars (description: > or |).
//  3. Neither present → trigger remains empty (caller emits missing-trigger warning).
func parseFrontmatterFields(block []byte) FrontmatterResult {
	var result FrontmatterResult
	lines := strings.Split(string(block), "\n")

	// descLines accumulates the description scalar value across folded/block lines.
	var descLines []string
	inDesc := false // true while inside a folded/block description scalar

	for _, line := range lines {
		// Normalize away a trailing \r so that CRLF files (\r\n split by \n
		// leaves a trailing \r) are treated identically to LF files. A blank
		// CRLF line becomes "\r" after the split; without stripping it has
		// len=1 and line[0]=='\r', which would wrongly terminate the folded
		// scalar. After stripping it becomes "" (empty), which is the correct
		// blank-line representation that continuation parsing expects.
		trimmedLine := strings.TrimRight(line, "\r")

		// A top-level key (no leading whitespace) ends any ongoing folded scalar.
		// An empty (blank) line does NOT end the scalar — it is a paragraph
		// separator within the folded value.
		if inDesc && len(trimmedLine) > 0 && trimmedLine[0] != ' ' && trimmedLine[0] != '\t' {
			inDesc = false
		}

		if inDesc {
			// Continuation line of a folded/block description scalar.
			descLines = append(descLines, strings.TrimSpace(trimmedLine))
			continue
		}

		key, value, ok := parseFrontmatterLine(trimmedLine)
		if !ok {
			continue
		}
		switch key {
		case "name":
			result.Name = value
		case "display_name":
			result.DisplayName = value
		case "trigger":
			result.Trigger = value
		case "scope":
			result.Scope = value
		case "description":
			if value == ">" || value == "|" {
				// Folded or literal block scalar — value follows on indented lines.
				inDesc = true
			} else {
				// Single-line (plain or quoted) description value.
				descLines = append(descLines, value)
			}
		}
	}

	// Capture the full description value (raw, before trigger extraction).
	if len(descLines) > 0 {
		result.Description = strings.Join(descLines, " ")
	}

	// Step 2: if no standalone trigger was found, extract from description.
	if result.Trigger == "" && len(descLines) > 0 {
		result.Trigger = extractTriggerFromDescription(result.Description)
	}

	return result
}

// extractTriggerFromDescription scans text for the first occurrence of
// "Trigger:" (case-insensitive) and returns the text that follows it,
// trimmed of surrounding whitespace and trailing YAML block-scalar artifacts.
// The FIRST "Trigger:" occurrence wins. The extracted value is terminated at
// the first sentence boundary after "Trigger:" — i.e. the first period that is
// immediately followed by a space or is at the end of the string. This prevents
// trailing summary prose (e.g. "Trigger: improve skills. Audit and upgrade...")
// from leaking into the trigger value.
// Returns an empty string when the marker is not found.
func extractTriggerFromDescription(text string) string {
	lower := strings.ToLower(text)
	idx := strings.Index(lower, "trigger:")
	if idx < 0 {
		return ""
	}
	after := strings.TrimSpace(text[idx+len("trigger:"):])
	// Strip stray surrounding quotes that survive quote-stripping on single-line values.
	after = strings.Trim(after, `"'`)
	after = strings.TrimSpace(after)
	// Truncate at the first sentence boundary: a period followed by a space or
	// at the very end of the string. This drops trailing summary prose that
	// sometimes follows the trigger sentence in folded/inline descriptions.
	for i := 0; i < len(after); i++ {
		if after[i] == '.' {
			// Period at end of string: include it and stop.
			if i == len(after)-1 {
				return after[:i+1]
			}
			// Period followed by whitespace: include the period and stop.
			if after[i+1] == ' ' || after[i+1] == '\t' || after[i+1] == '\n' {
				return after[:i+1]
			}
		}
	}
	return after
}

// parseFrontmatterLine parses a single "key: value" YAML line.
// Returns (key, value, true) on success; ("", "", false) otherwise.
// Keys are normalized to lowercase.
func parseFrontmatterLine(line string) (string, string, bool) {
	colonIdx := strings.IndexByte(line, ':')
	if colonIdx < 0 {
		return "", "", false
	}
	key := strings.ToLower(strings.TrimSpace(line[:colonIdx]))
	value := strings.TrimSpace(line[colonIdx+1:])
	// Strip optional inline YAML quotes
	value = strings.Trim(value, `"'`)
	if key == "" {
		return "", "", false
	}
	return key, value, true
}
