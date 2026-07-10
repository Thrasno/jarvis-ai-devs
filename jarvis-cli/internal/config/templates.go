// Templates provides rendering functions for CLAUDE.md and AGENTS.md from embedded templates.
// The embed.FS is provided by the caller (assets.TemplatesFS from the root package)
// via function parameters — this avoids invalid ".." paths in go:embed directives.
package config

import (
	"bytes"
	_ "embed"
	"fmt"
	"io/fs"
	"strings"
	"text/template"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

// SkillInfo represents skill metadata for template rendering.
type SkillInfo struct {
	Name        string
	Description string
	Trigger     string
}

// TemplateData holds the data passed to CLAUDE.md and AGENTS.md templates.
type TemplateData struct {
	Layer1    string
	Layer2    string
	Expertise string
	Skills    []SkillInfo
}

// RenderCLAUDEMd renders the CLAUDE.md content from the provided filesystem.
// fsys must contain "embed/templates/CLAUDE.md.tmpl" (root-package TemplatesFS layout).
// Accepts any fs.FS implementation, including embed.FS and fstest.MapFS.
func RenderCLAUDEMd(fsys fs.FS, layer1, layer2, expertise string, skills []SkillInfo) (string, error) {
	return renderTemplate(fsys, "embed/templates/CLAUDE.md.tmpl", TemplateData{
		Layer1:    layer1,
		Layer2:    layer2,
		Expertise: expertise,
		Skills:    skills,
	})
}

// RenderAGENTSMd renders the AGENTS.md content from the provided filesystem.
// fsys must contain "embed/templates/AGENTS.md.tmpl" (root-package TemplatesFS layout).
// Accepts any fs.FS implementation, including embed.FS and fstest.MapFS.
func RenderAGENTSMd(fsys fs.FS, layer1, layer2, expertise string, skills []SkillInfo) (string, error) {
	return renderTemplate(fsys, "embed/templates/AGENTS.md.tmpl", TemplateData{
		Layer1:    layer1,
		Layer2:    layer2,
		Expertise: expertise,
		Skills:    skills,
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
	return strings.TrimSpace(TechnicalContractContent() + "\n\n" + layer1Content)
}

// TechnicalContractContent returns the sole renderer-owned technical and
// educational policy source for generated instructions.
func TechnicalContractContent() string {
	return strings.TrimSpace(jarvis.TechnicalContract)
}

// InstructionProjection exposes the two renderer-owned instruction layers for
// effective-policy checks without comparing presentation bytes across surfaces.
type InstructionProjection struct {
	Layer1 string
	Layer2 string
}

// ProjectInstruction extracts Layer1 and Layer2 from a rendered instruction.
func ProjectInstruction(content string) (InstructionProjection, error) {
	layer1, err := projectMarkedContent(content, "<!-- JARVIS:LAYER1:START -->", "<!-- JARVIS:LAYER1:END -->")
	if err != nil {
		return InstructionProjection{}, fmt.Errorf("project Layer1: %w", err)
	}
	layer2, err := projectMarkedContent(content, "<!-- JARVIS:LAYER2:START -->", "<!-- JARVIS:LAYER2:END -->")
	if err != nil {
		return InstructionProjection{}, fmt.Errorf("project Layer2: %w", err)
	}
	return InstructionProjection{Layer1: layer1, Layer2: layer2}, nil
}

func projectMarkedContent(content, start, end string) (string, error) {
	startIndex := strings.Index(content, start)
	if startIndex == -1 {
		return "", fmt.Errorf("missing start marker %q", start)
	}
	bodyStart := startIndex + len(start)
	endIndex := strings.Index(content[bodyStart:], end)
	if endIndex == -1 {
		return "", fmt.Errorf("missing end marker %q", end)
	}
	return strings.TrimSpace(content[bodyStart : bodyStart+endIndex]), nil
}
