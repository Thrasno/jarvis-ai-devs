package sddspecsync

import "strings"

// heading is an ATX heading found outside fenced code blocks.
type heading struct {
	line  int
	level int
	text  string
}

// requirementBlock spans a "### Requirement: <name>" heading up to the next
// heading of level 1-3 (or EOF). contentEnd excludes trailing blank lines.
type requirementBlock struct {
	name       string
	start      int
	end        int
	contentEnd int
}

// document is LF-split Markdown with its structural headings resolved.
type document struct {
	lines      []string
	headings   []heading
	unclosed   bool
	duplicates []string
	blocks     []requirementBlock
}

const requirementPrefix = "Requirement:"

// normalizeNewlines converts CRLF line endings to LF.
func normalizeNewlines(data []byte) string {
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

func newDocument(lines []string) document {
	d := document{lines: lines}
	fenced, unclosed := fencedLines(lines)
	d.unclosed = unclosed
	for i, line := range lines {
		if fenced[i] {
			continue
		}
		if level, text, ok := atxHeading(line); ok {
			d.headings = append(d.headings, heading{line: i, level: level, text: text})
		}
	}
	seen := map[string]bool{}
	for i, h := range d.headings {
		name, ok := requirementName(h)
		if !ok {
			continue
		}
		end := len(lines)
		for _, next := range d.headings[i+1:] {
			if next.level <= 3 {
				end = next.line
				break
			}
		}
		if seen[name] {
			d.duplicates = append(d.duplicates, name)
		}
		seen[name] = true
		d.blocks = append(d.blocks, requirementBlock{name: name, start: h.line, end: end, contentEnd: lastContent(lines, h.line, end)})
	}
	return d
}

func (d document) find(name string) (requirementBlock, bool) {
	for _, b := range d.blocks {
		if b.name == name {
			return b, true
		}
	}
	return requirementBlock{}, false
}

// requirementName reports the name of a "### Requirement: <name>" heading.
func requirementName(h heading) (string, bool) {
	if h.level != 3 || !strings.HasPrefix(h.text, requirementPrefix) {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(h.text, requirementPrefix)), true
}

// lastContent returns the index just past the last non-blank line in
// lines[start:end], or start when the range is blank.
func lastContent(lines []string, start, end int) int {
	for i := end - 1; i >= start; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return i + 1
		}
	}
	return start
}

// atxHeading parses a CommonMark ATX heading (up to three spaces of indent,
// one to six '#', then a space or end of line, optional closing sequence).
func atxHeading(line string) (int, string, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return 0, "", false
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return 0, "", false
	}
	rest := trimmed[level:]
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return 0, "", false
	}
	text := strings.TrimSpace(rest)
	if closed := strings.TrimRight(text, "#"); closed != text && (closed == "" || strings.HasSuffix(closed, " ") || strings.HasSuffix(closed, "\t")) {
		text = strings.TrimSpace(closed)
	}
	return level, text, true
}

// fencedLines marks lines that belong to fenced code blocks, fences included,
// and reports whether a fence is still open at the end.
func fencedLines(lines []string) ([]bool, bool) {
	fenced := make([]bool, len(lines))
	var fenceChar byte
	fenceLen := 0
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		indented := len(line)-len(trimmed) > 3
		if fenceChar != 0 {
			fenced[i] = true
			if !indented && closesFence(trimmed, fenceChar, fenceLen) {
				fenceChar = 0
			}
			continue
		}
		if indented {
			continue
		}
		if char, n := opensFence(trimmed); n > 0 {
			fenced[i] = true
			fenceChar, fenceLen = char, n
		}
	}
	return fenced, fenceChar != 0
}

func opensFence(line string) (byte, int) {
	if line == "" || (line[0] != '`' && line[0] != '~') {
		return 0, 0
	}
	char := line[0]
	n := 0
	for n < len(line) && line[n] == char {
		n++
	}
	if n < 3 || (char == '`' && strings.ContainsRune(line[n:], '`')) {
		return 0, 0
	}
	return char, n
}

func closesFence(line string, char byte, minLen int) bool {
	n := 0
	for n < len(line) && line[n] == char {
		n++
	}
	return n >= minLen && strings.TrimSpace(line[n:]) == ""
}

// render joins lines with LF and ends the output with exactly one newline:
// trailing whitespace-only lines are dropped.
func render(lines []string) []byte {
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return []byte(strings.Join(lines[:end], "\n") + "\n")
}
