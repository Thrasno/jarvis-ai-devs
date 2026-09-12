package applyprogress

import (
	"strconv"
	"strings"
)

// ParsedTasks is the single interpretation of a frozen tasks.md artifact.
// Task text is preserved here; TaskManifest owns its normalization and digest.
type ParsedTasks struct {
	Tasks        []Task
	Completed    int
	CompletedIDs []string
	AllDone      bool
}

// ParseTasksMarkdown extracts deterministic repository implementation tasks from
// tasks.md. Checkbox rows in a Parent Actions section are parent prose and are
// ignored; all other malformed or non-repository task identifiers fail closed.
func ParseTasksMarkdown(content string) (ParsedTasks, error) {
	var parsed ParsedTasks
	seen := make(map[string]struct{})
	parentActionsDepth := 0
	for lineNumber, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if depth, heading, ok := markdownHeading(line); ok {
			if strings.Contains(strings.ToLower(heading), "parent action") {
				parentActionsDepth = depth
			} else if parentActionsDepth != 0 && depth <= parentActionsDepth {
				parentActionsDepth = 0
			}
			continue
		}
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		// Parent Actions is human workflow prose, not the authoritative task
		// manifest. Ignore every checkbox-shaped row in that section before
		// interpreting its checkbox or identifier.
		if parentActionsDepth != 0 {
			continue
		}
		if len(line) < 7 || line[4] != ']' || (line[3] != 'x' && line[3] != 'X' && line[3] != ' ') {
			return ParsedTasks{}, invalid(CodeInvalidTask, taskLineDetail(raw, lineNumber+1))
		}
		fields := strings.Fields(line[5:])
		if len(fields) == 0 {
			return ParsedTasks{}, invalid(CodeInvalidTask, taskLineDetail(raw, lineNumber+1))
		}
		if !taskMarkdownID(fields[0]) {
			return ParsedTasks{}, invalid(CodeInvalidTask, taskLineDetail(raw, lineNumber+1))
		}
		if _, duplicate := seen[fields[0]]; duplicate {
			return ParsedTasks{}, invalid(CodeInvalidTaskManifest, fields[0])
		}
		seen[fields[0]] = struct{}{}
		parsed.Tasks = append(parsed.Tasks, Task{ID: fields[0], Text: strings.Join(fields[1:], " ")})
		if line[3] == 'x' || line[3] == 'X' {
			parsed.Completed++
			parsed.CompletedIDs = append(parsed.CompletedIDs, fields[0])
		}
	}
	parsed.AllDone = len(parsed.Tasks) > 0 && parsed.Completed == len(parsed.Tasks)
	return parsed, nil
}

// ParseLegacyTasksMarkdown accepts the historical checkbox formats used before
// v2 required explicit task identifiers. It is deliberately separate from the
// strict v2 parser: callers may use it only to render legacy status or construct
// an authorized legacy migration.
func ParseLegacyTasksMarkdown(content string) (ParsedTasks, error) {
	var parsed ParsedTasks
	parentActionsDepth := 0
	section := "root"
	seen := make(map[string]struct{})
	seenPaths := make(map[string]struct{})
	for lineNumber, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if depth, heading, ok := markdownHeading(line); ok {
			if strings.Contains(strings.ToLower(heading), "parent action") {
				parentActionsDepth = depth
			} else {
				if parentActionsDepth != 0 && depth <= parentActionsDepth {
					parentActionsDepth = 0
				}
				section = heading
			}
			continue
		}
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		if parentActionsDepth != 0 {
			continue
		}
		if len(line) < 7 || line[4] != ']' || (line[3] != 'x' && line[3] != 'X' && line[3] != ' ') {
			return ParsedTasks{}, invalid(CodeInvalidTask, taskLineDetail(raw, lineNumber+1))
		}
		path, text, err := parseLegacyTaskRow(section, strings.TrimSpace(line[5:]))
		if err != nil {
			return ParsedTasks{}, invalid(CodeInvalidTask, taskLineDetail(raw, lineNumber+1))
		}
		id, err := LegacyTaskID(path, text)
		if err != nil {
			return ParsedTasks{}, invalid(CodeInvalidTask, taskLineDetail(raw, lineNumber+1))
		}
		if _, duplicate := seen[id]; duplicate {
			return ParsedTasks{}, invalid(CodeInvalidTaskManifest, id)
		}
		if _, duplicate := seenPaths[path]; duplicate {
			return ParsedTasks{}, invalid(CodeInvalidTaskManifest, path)
		}
		seen[id], seenPaths[path] = struct{}{}, struct{}{}
		parsed.Tasks = append(parsed.Tasks, Task{ID: id, Path: path, Text: text})
		if line[3] == 'x' || line[3] == 'X' {
			parsed.Completed++
			parsed.CompletedIDs = append(parsed.CompletedIDs, path)
		}
	}
	parsed.AllDone = len(parsed.Tasks) > 0 && parsed.Completed == len(parsed.Tasks)
	return parsed, nil
}

func parseLegacyTaskRow(section, row string) (path, text string, err error) {
	fields := strings.Fields(row)
	if len(fields) == 0 {
		return "", "", ErrInvalidValue
	}
	if taskMarkdownID(fields[0]) && len(fields) > 1 {
		return fields[0], strings.Join(fields[1:], " "), nil
	}
	phase := strings.Trim(strings.Trim(fields[0], "*"), ":-")
	switch strings.ToUpper(phase) {
	case "RED", "GREEN", "TRIANGULATE", "REFACTOR":
		text = strings.TrimSpace(row)
		normalized, normalizeErr := NormalizeTaskText(text)
		if normalizeErr != nil {
			return "", "", normalizeErr
		}
		// A frozen section path plus task text is stable across migration retries.
		// The generated path stays inside the ordinary protocol ID domain.
		return "legacy-path-" + digest([]byte(section + "\n" + normalized))[:32], normalized, nil
	default:
		return "", "", ErrInvalidValue
	}
}

func taskLineDetail(_ string, line int) string { return "line " + strconv.Itoa(line) }

func taskMarkdownID(id string) bool {
	if id == "" || id[len(id)-1] == '.' {
		return false
	}
	// Established task lists use numeric namespaces (1A.2, 2.3) and
	// letter-prefixed namespaces (C2.1, R1). This parser intentionally remains
	// narrower than the generic protocol identifier validator.
	if id[0] >= 'A' && id[0] <= 'Z' {
		if len(id) == 1 || id[1] < '0' || id[1] > '9' {
			return false
		}
	} else if id[0] < '0' || id[0] > '9' {
		return false
	}
	componentStart := false
	for i := 0; i < len(id); i++ {
		if id[i] == '.' {
			if componentStart {
				return false
			}
			componentStart = true
			continue
		}
		if (id[i] < '0' || id[i] > '9') && (id[i] < 'A' || id[i] > 'Z') && (id[i] < 'a' || id[i] > 'z') {
			return false
		}
		componentStart = false
	}
	return true
}

// markdownHeading returns the ATX heading depth and body. Parent Actions is a
// section, so nested headings remain inside it until a same-or-higher heading.
func markdownHeading(line string) (int, string, bool) {
	if line == "" || line[0] != '#' {
		return 0, "", false
	}
	depth := 0
	for depth < len(line) && line[depth] == '#' {
		depth++
	}
	if depth == 0 || depth == len(line) || (line[depth] != ' ' && line[depth] != '\t') {
		return 0, "", false
	}
	return depth, strings.TrimSpace(line[depth:]), true
}
