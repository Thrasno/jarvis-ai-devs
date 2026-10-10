package sddspecsync

import (
	"strings"
	"unicode"
)

// Delta section names as written in "## <NAME> Requirements" headings.
const (
	SectionAdded    = "ADDED"
	SectionModified = "MODIFIED"
	SectionRemoved  = "REMOVED"
	SectionRenamed  = "RENAMED"
)

// applyOrder is the order sections are applied in: renames first so MODIFIED
// blocks address requirements by their new name, additions last.
var applyOrder = []string{SectionRenamed, SectionRemoved, SectionModified, SectionAdded}

// Rename is one RENAMED requirement.
type Rename struct {
	From string
	To   string
}

// Changes summarizes the requirement operations a merge performed, in delta
// order per section.
type Changes struct {
	Added         []string
	Modified      []string
	Removed       []string
	Renamed       []Rename
	NewCapability bool
}

// Destructive reports whether the merge deletes requirements.
func (c Changes) Destructive() bool { return len(c.Removed) > 0 }

// deltaBlock is one requirement block of a delta section, trailing blank
// lines trimmed.
type deltaBlock struct {
	name  string
	lines []string
}

type deltaSpec map[string][]deltaBlock

// MergeSpec merges a delta spec into a main spec and returns the new main
// spec. A nil main means the capability has no main spec yet: the delta must
// then be a full spec and is copied as is.
//
// Input CRLF line endings are normalized to LF. The output always uses LF and
// ends with exactly one newline; every line outside the touched requirement
// blocks is preserved byte for byte.
func MergeSpec(main, delta []byte) ([]byte, error) {
	out, _, err := merge(main, main != nil, delta)
	return out, err
}

func merge(main []byte, exists bool, delta []byte) ([]byte, Changes, error) {
	deltaText := normalizeNewlines(delta)
	if strings.TrimSpace(deltaText) == "" {
		return nil, Changes{}, &MergeError{Kind: ErrInvalidDelta, Detail: "delta spec is blank"}
	}
	if !exists {
		out, err := copyNewCapability(deltaText)
		if err != nil {
			return nil, Changes{}, err
		}
		return out, Changes{NewCapability: true}, nil
	}
	spec, err := parseDelta(deltaText)
	if err != nil {
		return nil, Changes{}, err
	}
	if err := validateDelta(spec); err != nil {
		return nil, Changes{}, err
	}
	doc := newDocument(strings.Split(normalizeNewlines(main), "\n"))
	if doc.unclosed {
		return nil, Changes{}, &MergeError{Kind: ErrInvalidMainSpec, Detail: "unclosed fenced code block"}
	}
	if len(doc.duplicates) > 0 {
		return nil, Changes{}, &MergeError{Kind: ErrInvalidMainSpec, Requirement: doc.duplicates[0], Detail: "requirement name appears more than once"}
	}
	return applyDelta(doc, spec)
}

// copyNewCapability validates a full spec for a capability without a main
// spec. It may not modify, remove, or rename anything.
func copyNewCapability(text string) ([]byte, error) {
	doc := newDocument(strings.Split(text, "\n"))
	if doc.unclosed {
		return nil, &MergeError{Kind: ErrInvalidDelta, Detail: "unclosed fenced code block"}
	}
	for _, h := range doc.headings {
		if section, ok := sectionOf(h); ok && section != SectionAdded {
			return nil, &MergeError{Kind: ErrInvalidDelta, Section: section, Detail: "new capability has no main spec to change"}
		}
	}
	if len(doc.duplicates) > 0 {
		return nil, &MergeError{Kind: ErrDuplicateRequirement, Requirement: doc.duplicates[0], Detail: "requirement name appears more than once"}
	}
	if len(doc.blocks) == 0 {
		return nil, &MergeError{Kind: ErrInvalidDelta, Detail: "new capability spec has no requirements"}
	}
	for _, b := range doc.blocks {
		if b.name == "" {
			return nil, &MergeError{Kind: ErrInvalidDelta, Detail: "requirement heading without a name"}
		}
	}
	return render(doc.lines), nil
}

func sectionOf(h heading) (string, bool) {
	if h.level != 2 {
		return "", false
	}
	for _, section := range applyOrder {
		if h.text == section+" Requirements" {
			return section, true
		}
	}
	return "", false
}

// parseDelta splits a delta spec into its operation sections. Only a level-1
// title and free prose may precede the first section; inside sections every
// non-blank line must belong to a "### Requirement:" block.
func parseDelta(text string) (deltaSpec, error) {
	doc := newDocument(strings.Split(text, "\n"))
	if doc.unclosed {
		return nil, &MergeError{Kind: ErrInvalidDelta, Detail: "unclosed fenced code block"}
	}
	headingAt := map[int]heading{}
	for _, h := range doc.headings {
		headingAt[h.line] = h
	}
	spec := deltaSpec{}
	section := ""
	var current *deltaBlock
	closeBlock := func() {
		if current != nil {
			current.lines = current.lines[:lastContent(current.lines, 0, len(current.lines))]
			spec[section] = append(spec[section], *current)
			current = nil
		}
	}
	for i, line := range doc.lines {
		h, isHeading := headingAt[i]
		switch {
		case isHeading && h.level == 1:
			if section != "" {
				return nil, &MergeError{Kind: ErrInvalidDelta, Section: section, Detail: "level-1 heading inside delta sections"}
			}
		case isHeading && h.level == 2:
			closeBlock()
			next, ok := sectionOf(h)
			if !ok {
				return nil, &MergeError{Kind: ErrInvalidDelta, Detail: "unsupported section " + quote(h.text)}
			}
			if _, seen := spec[next]; seen {
				return nil, &MergeError{Kind: ErrInvalidDelta, Section: next, Detail: "section appears more than once"}
			}
			section = next
			spec[section] = nil
		case isHeading && h.level == 3:
			closeBlock()
			name, ok := requirementName(h)
			if !ok {
				return nil, &MergeError{Kind: ErrInvalidDelta, Section: section, Detail: "unsupported heading " + quote(h.text)}
			}
			if section == "" {
				return nil, &MergeError{Kind: ErrInvalidDelta, Requirement: name, Detail: "requirement outside an ADDED/MODIFIED/REMOVED/RENAMED section"}
			}
			if name == "" {
				return nil, &MergeError{Kind: ErrInvalidDelta, Section: section, Detail: "requirement heading without a name"}
			}
			current = &deltaBlock{name: name, lines: []string{line}}
		case current != nil:
			current.lines = append(current.lines, line)
		case isHeading:
			return nil, &MergeError{Kind: ErrInvalidDelta, Section: section, Detail: "heading " + quote(h.text) + " outside a requirement block"}
		case section != "" && strings.TrimSpace(line) != "":
			return nil, &MergeError{Kind: ErrInvalidDelta, Section: section, Detail: "content outside a requirement block"}
		}
	}
	closeBlock()
	if len(spec) == 0 {
		return nil, &MergeError{Kind: ErrInvalidDelta, Detail: "delta has no ADDED/MODIFIED/REMOVED/RENAMED sections"}
	}
	for _, section := range applyOrder {
		if blocks, ok := spec[section]; ok && len(blocks) == 0 {
			return nil, &MergeError{Kind: ErrInvalidDelta, Section: section, Detail: "section has no requirements"}
		}
	}
	return spec, nil
}

// validateDelta checks per-block evidence and cross-section consistency
// before anything touches the main spec.
func validateDelta(spec deltaSpec) error {
	for _, section := range applyOrder {
		seen := map[string]bool{}
		for _, b := range spec[section] {
			if seen[b.name] {
				return &MergeError{Kind: ErrInvalidDelta, Section: section, Requirement: b.name, Detail: "requirement appears more than once in the section"}
			}
			seen[b.name] = true
		}
	}
	for _, b := range spec[SectionRemoved] {
		if err := validateRemoval(b); err != nil {
			return err
		}
	}
	renames := map[string]string{}
	renamedTo := map[string]bool{}
	for _, b := range spec[SectionRenamed] {
		r, err := parseRename(b)
		if err != nil {
			return err
		}
		if _, dup := renames[r.From]; dup {
			return &MergeError{Kind: ErrInvalidDelta, Section: SectionRenamed, Requirement: r.From, Detail: "requirement renamed more than once"}
		}
		renames[r.From] = r.To
		renamedTo[r.To] = true
	}
	for from := range renames {
		if renamedTo[from] {
			return &MergeError{Kind: ErrInvalidDelta, Section: SectionRenamed, Requirement: from, Detail: "chained or swapped renames are not supported"}
		}
	}
	owner := map[string]string{}
	for _, section := range []string{SectionRemoved, SectionModified, SectionAdded} {
		for _, b := range spec[section] {
			if other, ok := owner[b.name]; ok {
				return &MergeError{Kind: ErrInvalidDelta, Section: section, Requirement: b.name, Detail: "requirement also appears in " + other}
			}
			owner[b.name] = section
			if _, ok := renames[b.name]; ok {
				return &MergeError{Kind: ErrInvalidDelta, Section: section, Requirement: b.name, Detail: "requirement is renamed; refer to it by its new name"}
			}
			if renamedTo[b.name] && section != SectionModified {
				return &MergeError{Kind: ErrInvalidDelta, Section: section, Requirement: b.name, Detail: "requirement is the target of a rename"}
			}
		}
	}
	return nil
}

func validateRemoval(b deltaBlock) error {
	meta, _, err := blockMetadata(b, SectionRemoved, ErrRemovalEvidence)
	if err != nil {
		return err
	}
	reason, hasReason := meta["reason"]
	migration, hasMigration := meta["migration"]
	fail := func(detail string) error {
		return &MergeError{Kind: ErrRemovalEvidence, Section: SectionRemoved, Requirement: b.name, Detail: detail}
	}
	switch {
	case !hasReason || isPlaceholder(reason):
		return fail("Reason is missing, empty, or a placeholder")
	case !hasMigration:
		return fail("Migration is missing")
	case !validMigration(migration):
		return fail("Migration is empty, a placeholder, or None without justification")
	}
	return nil
}

func parseRename(b deltaBlock) (Rename, error) {
	meta, other, err := blockMetadata(b, SectionRenamed, ErrRenameEvidence)
	if err != nil {
		return Rename{}, err
	}
	fail := func(kind error, detail string) error {
		return &MergeError{Kind: kind, Section: SectionRenamed, Requirement: b.name, Detail: detail}
	}
	if other {
		return Rename{}, fail(ErrInvalidDelta, "RENAMED blocks only carry Old name/New name/Reason; use MODIFIED to change the body")
	}
	from, hasFrom := meta["old name"]
	to, hasTo := meta["new name"]
	switch {
	case !hasFrom || isPlaceholder(from):
		return Rename{}, fail(ErrRenameEvidence, "Old name is missing or a placeholder")
	case !hasTo || isPlaceholder(to):
		return Rename{}, fail(ErrRenameEvidence, "New name is missing or a placeholder")
	case to != b.name:
		return Rename{}, fail(ErrRenameEvidence, "New name "+quote(to)+" does not match the heading")
	case from == to:
		return Rename{}, fail(ErrRenameEvidence, "Old name and New name are identical")
	}
	if reason, ok := meta["reason"]; ok && isPlaceholder(reason) {
		return Rename{}, fail(ErrRenameEvidence, "Reason is a placeholder")
	}
	return Rename{From: from, To: to}, nil
}

// blockMetadata collects "(Key: value)" lines of a delta block, outside
// fenced code. It reports whether the block holds any other non-blank content
// and rejects repeated keys.
func blockMetadata(b deltaBlock, section string, kind error) (map[string]string, bool, error) {
	meta := map[string]string{}
	other := false
	fenced, _ := fencedLines(b.lines)
	for i, line := range b.lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, ok := metadataLine(line)
		if !ok || fenced[i+1] {
			other = true
			continue
		}
		if _, dup := meta[key]; dup {
			return nil, false, &MergeError{Kind: kind, Section: section, Requirement: b.name, Detail: "repeated " + quote(key) + " evidence"}
		}
		meta[key] = value
	}
	return meta, other, nil
}

var metadataKeys = []string{"reason", "migration", "old name", "new name"}

// metadataLine parses "(Key: value)" or "Key: value" for a known key.
func metadataLine(line string) (string, string, bool) {
	s := strings.TrimSpace(line)
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	key, value, ok := strings.Cut(s, ":")
	if !ok {
		return "", "", false
	}
	key = strings.ToLower(strings.TrimSpace(key))
	for _, known := range metadataKeys {
		if key == known {
			return key, strings.TrimSpace(value), true
		}
	}
	return "", "", false
}

var placeholderValues = map[string]bool{
	"": true, "tbd": true, "tba": true, "todo": true, "fixme": true, "xxx": true,
	"n/a": true, "na": true, "none": true, "-": true, "?": true, "...": true, "…": true,
	"placeholder": true, "pending": true, "unknown": true,
}

// isPlaceholder reports empty or template evidence such as "{why}", "TBD",
// "N/A", or a bare "None".
func isPlaceholder(value string) bool {
	v := strings.Trim(strings.TrimSpace(value), "`*_")
	if (strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}")) ||
		(strings.HasPrefix(v, "<") && strings.HasSuffix(v, ">")) ||
		(strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]")) {
		return true
	}
	v = strings.TrimRightFunc(v, unicode.IsPunct)
	return placeholderValues[strings.ToLower(strings.TrimFunc(v, unicode.IsSpace))] ||
		strings.TrimFunc(v, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) }) == ""
}

// validMigration accepts concrete migration evidence, or "None" followed by a
// non-placeholder justification.
func validMigration(value string) bool {
	s := strings.TrimLeft(strings.TrimSpace(value), "`")
	if len(s) >= 4 && strings.EqualFold(s[:4], "none") && (len(s) == 4 || !isWordRune(rune(s[4]))) {
		justification := strings.TrimLeftFunc(s[4:], func(r rune) bool {
			return unicode.IsSpace(r) || unicode.IsPunct(r) || r == '`'
		})
		return !isPlaceholder(justification)
	}
	return !isPlaceholder(value)
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// applyDelta applies the validated delta to the main spec in applyOrder.
func applyDelta(doc document, spec deltaSpec) ([]byte, Changes, error) {
	var changes Changes
	fail := func(kind error, section, name, detail string) ([]byte, Changes, error) {
		return nil, Changes{}, &MergeError{Kind: kind, Section: section, Requirement: name, Detail: detail}
	}
	for _, b := range spec[SectionRenamed] {
		r, _ := parseRename(b)
		target, ok := doc.find(r.From)
		if !ok {
			return fail(ErrRequirementNotFound, SectionRenamed, r.From, "no requirement to rename")
		}
		if _, exists := doc.find(r.To); exists {
			return fail(ErrDuplicateRequirement, SectionRenamed, r.To, "rename target already exists")
		}
		lines := append([]string(nil), doc.lines...)
		lines[target.start] = "### " + requirementPrefix + " " + r.To
		doc = newDocument(lines)
		changes.Renamed = append(changes.Renamed, r)
	}
	for _, b := range spec[SectionRemoved] {
		target, ok := doc.find(b.name)
		if !ok {
			return fail(ErrRequirementNotFound, SectionRemoved, b.name, "no requirement to remove")
		}
		doc = newDocument(splice(doc.lines, target.start, target.end, nil))
		changes.Removed = append(changes.Removed, b.name)
	}
	for _, b := range spec[SectionModified] {
		target, ok := doc.find(b.name)
		if !ok {
			return fail(ErrRequirementNotFound, SectionModified, b.name, "no requirement to modify")
		}
		doc = newDocument(splice(doc.lines, target.start, target.contentEnd, b.lines))
		changes.Modified = append(changes.Modified, b.name)
	}
	for _, b := range spec[SectionAdded] {
		if _, exists := doc.find(b.name); exists {
			return fail(ErrDuplicateRequirement, SectionAdded, b.name, "requirement already exists in the main spec")
		}
		start, end, err := requirementsSection(doc)
		if err != nil {
			return nil, Changes{}, err
		}
		last := lastContent(doc.lines, start, end)
		trailer := append([]string(nil), doc.lines[last:end]...)
		if len(trailer) == 0 && end < len(doc.lines) {
			trailer = []string{""}
		}
		insert := append(append([]string{""}, b.lines...), trailer...)
		doc = newDocument(splice(doc.lines, last, end, insert))
		changes.Added = append(changes.Added, b.name)
	}
	return render(doc.lines), changes, nil
}

// requirementsSection locates the section ADDED requirements are appended
// to: the unique "## Requirements" section or, for main specs that were
// copied from an ADDED-only delta, the unique "## ADDED Requirements" one.
func requirementsSection(doc document) (int, int, error) {
	var requirements, added []heading
	for _, h := range doc.headings {
		switch {
		case h.level == 2 && h.text == "Requirements":
			requirements = append(requirements, h)
		case h.level == 2 && h.text == SectionAdded+" Requirements":
			added = append(added, h)
		}
	}
	var section heading
	switch {
	case len(requirements) == 1 && len(added) == 0:
		section = requirements[0]
	case len(requirements) == 0 && len(added) == 1:
		section = added[0]
	default:
		return 0, 0, &MergeError{Kind: ErrNoRequirementsSection, Section: SectionAdded, Detail: "expected exactly one \"## Requirements\" section"}
	}
	end := len(doc.lines)
	for _, h := range doc.headings {
		if h.line > section.line && h.level <= 2 {
			end = h.line
			break
		}
	}
	return section.line, end, nil
}

func splice(lines []string, start, end int, replacement []string) []string {
	out := make([]string, 0, len(lines)-(end-start)+len(replacement))
	out = append(out, lines[:start]...)
	out = append(out, replacement...)
	return append(out, lines[end:]...)
}

func quote(s string) string { return "\"" + s + "\"" }
