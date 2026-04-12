// Templates provides rendering functions for CLAUDE.md and AGENTS.md from embedded templates.
// The embed.FS is provided by the caller (assets.TemplatesFS from the root package)
// via function parameters — this avoids invalid ".." paths in go:embed directives.
package config

import (
	"bytes"
	_ "embed"
	"fmt"
	"io/fs"
	"text/template"
)

// TemplateData holds the data passed to CLAUDE.md and AGENTS.md templates.
type TemplateData struct {
	Layer1    string
	Layer2    string
	Expertise string
}

// RenderCLAUDEMd renders the CLAUDE.md content from the provided filesystem.
// fsys must contain "embed/templates/CLAUDE.md.tmpl" (root-package TemplatesFS layout).
// Accepts any fs.FS implementation, including embed.FS and fstest.MapFS.
func RenderCLAUDEMd(fsys fs.FS, layer1, layer2, expertise string) (string, error) {
	return renderTemplate(fsys, "embed/templates/CLAUDE.md.tmpl", TemplateData{
		Layer1:    layer1,
		Layer2:    layer2,
		Expertise: expertise,
	})
}

// RenderAGENTSMd renders the AGENTS.md content from the provided filesystem.
// fsys must contain "embed/templates/AGENTS.md.tmpl" (root-package TemplatesFS layout).
// Accepts any fs.FS implementation, including embed.FS and fstest.MapFS.
func RenderAGENTSMd(fsys fs.FS, layer1, layer2, expertise string) (string, error) {
	return renderTemplate(fsys, "embed/templates/AGENTS.md.tmpl", TemplateData{
		Layer1:    layer1,
		Layer2:    layer2,
		Expertise: expertise,
	})
}

// renderTemplate renders a named template from the provided fs.FS.
func renderTemplate(fsys fs.FS, path string, data TemplateData) (string, error) {
	tmplBytes, err := fs.ReadFile(fsys, path)
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

//go:embed layer1.md
var layer1Content string

// Layer1Content returns the standard Layer1 Hive memory protocol content.
// The content is embedded at compile time from internal/config/layer1.md.
// This is the immutable content written between the LAYER1 sentinel markers.
func Layer1Content() string {
	return layer1Content
}
