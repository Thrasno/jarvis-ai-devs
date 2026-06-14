package projectregistry

import "strings"

func FormatWarningLine(warning Warning) string {
	message := formatWarningMessage(warning)
	if message == "" {
		return ""
	}
	return "Warning: " + message
}

func FormatWarningLines(prefix string, warnings []Warning) []string {
	lines := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		message := formatWarningMessage(warning)
		if message == "" {
			continue
		}
		lines = append(lines, prefix+message)
	}
	return lines
}

func formatWarningMessage(warning Warning) string {
	message := strings.TrimSpace(warning.Message)
	if message == "" {
		message = strings.TrimSpace(warning.Code)
	}
	if path := strings.TrimSpace(warning.Path); path != "" {
		message += " (" + path + ")"
	}
	return message
}
