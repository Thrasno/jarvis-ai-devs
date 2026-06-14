package sddruntime

import (
	"fmt"
	"strings"
)

type ModelSectionClass string

const (
	ModelSectionUnknown ModelSectionClass = "unknown"
	ModelSectionCapable ModelSectionClass = "capable"
	ModelSectionSmall   ModelSectionClass = "small"
)

const modelSectionOpenPrefix = "<!-- section:model-"

// ModelSectionClassForModel maps Jarvis runtime model assignments to the model
// section class used by rendered prompt assets. Jarvis aliases are authoritative:
// opus and sonnet are capable; haiku is small. Known provider-qualified model
// families are classified deterministically, and unrecognized models are unknown.
func ModelSectionClassForModel(model string) ModelSectionClass {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return ModelSectionUnknown
	}

	provider, modelID, providerQualified := strings.Cut(normalized, "/")
	if !providerQualified {
		return modelSectionClassForUnqualifiedModel(normalized)
	}

	provider = strings.TrimSpace(provider)
	modelID = strings.TrimSpace(modelID)
	switch provider {
	case "anthropic":
		return claudeModelSectionClass(modelID)
	case "openai":
		return openAIModelSectionClass(modelID)
	default:
		return ModelSectionUnknown
	}
}

func modelSectionClassForUnqualifiedModel(model string) ModelSectionClass {
	switch model {
	case "opus", "sonnet":
		return ModelSectionCapable
	case "haiku":
		return ModelSectionSmall
	}

	if class := claudeModelSectionClass(model); class != ModelSectionUnknown {
		return class
	}
	return openAIModelSectionClass(model)
}

func claudeModelSectionClass(model string) ModelSectionClass {
	switch {
	case model == "haiku" || strings.HasPrefix(model, "claude-haiku"):
		return ModelSectionSmall
	case model == "opus" || model == "sonnet" || strings.HasPrefix(model, "claude-opus") || strings.HasPrefix(model, "claude-sonnet"):
		return ModelSectionCapable
	default:
		return ModelSectionUnknown
	}
}

func openAIModelSectionClass(model string) ModelSectionClass {
	if !hasOpenAIModelFamilyPrefix(model) {
		return ModelSectionUnknown
	}
	if hasBoundedToken(model, "mini") || hasBoundedToken(model, "nano") || hasBoundedToken(model, "small") {
		return ModelSectionSmall
	}
	return ModelSectionCapable
}

func hasOpenAIModelFamilyPrefix(model string) bool {
	if hasFamilyPrefix(model, "gpt-5") || hasFamilyPrefix(model, "gpt-4") {
		return true
	}
	return hasFamilyPrefix(model, "o3") || hasFamilyPrefix(model, "o4")
}

func hasFamilyPrefix(model, family string) bool {
	if model == family || strings.HasPrefix(model, family+"-") || strings.HasPrefix(model, family+".") || strings.HasPrefix(model, family+"_") {
		return true
	}
	return family == "gpt-4" && (model == "gpt-4o" || strings.HasPrefix(model, "gpt-4o-") || strings.HasPrefix(model, "gpt-4o.") || strings.HasPrefix(model, "gpt-4o_"))
}

func hasBoundedToken(value, token string) bool {
	start := 0
	for start <= len(value) {
		idx := strings.Index(value[start:], token)
		if idx == -1 {
			return false
		}
		idx += start
		end := idx + len(token)
		if isTokenBoundary(value, idx-1) && isTokenBoundary(value, end) {
			return true
		}
		start = end
	}
	return false
}

func isTokenBoundary(value string, idx int) bool {
	if idx < 0 || idx >= len(value) {
		return true
	}
	c := value[idx]
	return c == '-' || c == '_' || c == '.' || c == '/'
}

// RenderModelSections processes Jarvis model-section markers in prompt assets.
// Matching sections are included without their marker comments. Non-matching
// sections are removed from rendered output. Unknown model class keeps only
// neutral content outside model-specific sections.
func RenderModelSections(content string, class ModelSectionClass) (string, error) {
	var out strings.Builder
	remaining := content

	for {
		start := strings.Index(remaining, modelSectionOpenPrefix)
		if start == -1 {
			out.WriteString(remaining)
			break
		}

		out.WriteString(remaining[:start])
		openEnd := strings.Index(remaining[start:], " -->")
		if openEnd == -1 {
			return "", fmt.Errorf("unterminated model section opening marker")
		}
		openEnd += start

		sectionName := remaining[start+len(modelSectionOpenPrefix) : openEnd]
		closeMarker := "<!-- /section:model-" + sectionName + " -->"
		bodyStart := openEnd + len(" -->")
		closeStart := strings.Index(remaining[bodyStart:], closeMarker)
		if closeStart == -1 {
			return "", fmt.Errorf("missing closing marker for model section %q", sectionName)
		}
		closeStart += bodyStart

		if class == ModelSectionClass(sectionName) {
			out.WriteString(trimSectionBody(remaining[bodyStart:closeStart]))
		}

		remaining = remaining[closeStart+len(closeMarker):]
	}

	return out.String(), nil
}

func trimSectionBody(body string) string {
	body = strings.TrimLeft(body, " \t\r\n")
	body = strings.TrimRight(body, " \t\r\n")
	if body == "" {
		return body
	}
	return body + "\n"
}
