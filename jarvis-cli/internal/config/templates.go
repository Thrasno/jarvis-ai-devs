// Templates provides rendering functions for CLAUDE.md and AGENTS.md from embedded templates.
// The embed.FS is provided by the caller (assets.TemplatesFS from the root package)
// via function parameters — this avoids invalid ".." paths in go:embed directives.
package config

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

// TemplateData holds the data passed to CLAUDE.md and AGENTS.md templates.
type TemplateData struct {
	Layer1    string
	Layer2    string
	Expertise string
}

// RenderCLAUDEMd renders the CLAUDE.md content from the embedded template.
// fsys must be the root-package TemplatesFS (embed/templates embedded at root).
func RenderCLAUDEMd(fsys embed.FS, layer1, layer2, expertise string) (string, error) {
	return renderTemplate(fsys, "embed/templates/CLAUDE.md.tmpl", TemplateData{
		Layer1:    layer1,
		Layer2:    layer2,
		Expertise: expertise,
	})
}

// RenderAGENTSMd renders the AGENTS.md content from the embedded template.
// fsys must be the root-package TemplatesFS (embed/templates embedded at root).
func RenderAGENTSMd(fsys embed.FS, layer1, layer2, expertise string) (string, error) {
	return renderTemplate(fsys, "embed/templates/AGENTS.md.tmpl", TemplateData{
		Layer1:    layer1,
		Layer2:    layer2,
		Expertise: expertise,
	})
}

// renderTemplate renders a named template from the provided embed.FS.
func renderTemplate(fsys embed.FS, path string, data TemplateData) (string, error) {
	tmplBytes, err := fsys.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read template %s: %w", path, err)
	}

	tmpl, err := template.New(path).Parse(string(tmplBytes))
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", path, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render template %s: %w", path, err)
	}

	return buf.String(), nil
}

// Layer1Content returns the standard Layer1 Hive memory protocol content.
// This is the immutable content written between the LAYER1 sentinel markers.
func Layer1Content() string {
	return `## Hive Persistent Memory — Protocol

You have access to Hive, a persistent memory system via MCP tools.
This protocol is MANDATORY and ALWAYS ACTIVE.

### PROACTIVE SAVE TRIGGERS (mandatory — do NOT wait for user to ask)

Call ` + "`mem_save`" + ` IMMEDIATELY and WITHOUT BEING ASKED after any of these:
- Architecture or design decision made
- Bug fix completed (include root cause)
- Feature implemented with non-obvious approach
- Non-obvious discovery about the codebase
- Pattern established (naming, structure, convention)
- Team convention documented

### Format for mem_save
- **title**: Verb + what (e.g. "Fixed N+1 query in UserList")
- **type**: bugfix | decision | architecture | discovery | pattern | config
- **topic_key**: stable key like ` + "`architecture/auth-model`" + `
- **content**: What / Why / Where / Learned

### WHEN TO SEARCH MEMORY
On any "remember", "recall", "what did we do", "how did we solve":
1. Call ` + "`mem_context`" + ` — recent session history
2. If not found, call ` + "`mem_search`" + ` with keywords
3. If found, use ` + "`mem_get_observation`" + ` for full content

### SDD ENFORCEMENT
- Never skip SDD phases for non-trivial changes
- Always use conventional commits format
- Never add Co-Authored-By AI attribution to commits

### WORKFLOW RULES
- ALWAYS verify before confirming technical claims
- WHEN asking a question: STOP and wait for response
- NEVER agree with user claims without checking code/docs first`
}
