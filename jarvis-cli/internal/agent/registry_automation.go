package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const jarvisExecutablePlaceholder = "{{JARVIS_EXECUTABLE}}"

const (
	registryAutomationTimeoutSeconds = 3
	registryAutomationTimeoutMillis  = registryAutomationTimeoutSeconds * 1000

	jarvisRefreshTimeoutSecondsPlaceholder = "{{JARVIS_REFRESH_TIMEOUT_SECONDS}}"
	jarvisRefreshTimeoutMillisPlaceholder  = "{{JARVIS_REFRESH_TIMEOUT_MILLIS}}"
)

func renderRegistryAutomationAsset(assetPath string, content []byte) ([]byte, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve jarvis executable: %w", err)
	}
	return renderRegistryAutomationAssetWithExecutable(assetPath, content, executable)
}

func renderRegistryAutomationAssetWithExecutable(assetPath string, content []byte, executable string) ([]byte, error) {
	if strings.TrimSpace(executable) == "" || !filepath.IsAbs(executable) {
		return nil, fmt.Errorf("resolve jarvis executable: expected absolute path, got %q", executable)
	}
	text := string(content)
	for _, placeholder := range expectedRegistryAutomationPlaceholders(assetPath) {
		if !strings.Contains(text, placeholder) {
			return nil, fmt.Errorf("render registry automation asset %s: missing placeholder %s", assetPath, placeholder)
		}
	}

	replacement := executableReplacementForAsset(assetPath, executable)
	replacer := strings.NewReplacer(
		jarvisExecutablePlaceholder, replacement,
		jarvisRefreshTimeoutSecondsPlaceholder, fmt.Sprintf("%d", registryAutomationTimeoutSeconds),
		jarvisRefreshTimeoutMillisPlaceholder, fmt.Sprintf("%d", registryAutomationTimeoutMillis),
	)
	rendered := replacer.Replace(text)
	for _, placeholder := range []string{jarvisExecutablePlaceholder, jarvisRefreshTimeoutSecondsPlaceholder, jarvisRefreshTimeoutMillisPlaceholder} {
		if strings.Contains(rendered, placeholder) {
			return nil, fmt.Errorf("render registry automation asset %s: unconsumed placeholder %s", assetPath, placeholder)
		}
	}
	return []byte(rendered), nil
}

func expectedRegistryAutomationPlaceholders(assetPath string) []string {
	placeholders := []string{jarvisExecutablePlaceholder}
	switch {
	case strings.HasSuffix(assetPath, ".sh"):
		placeholders = append(placeholders, jarvisRefreshTimeoutSecondsPlaceholder)
	case strings.HasSuffix(assetPath, ".ps1"), strings.HasSuffix(assetPath, ".ts"):
		placeholders = append(placeholders, jarvisRefreshTimeoutMillisPlaceholder)
	}
	return placeholders
}

func executableReplacementForAsset(assetPath, executable string) string {
	switch {
	case strings.HasSuffix(assetPath, ".ps1"):
		return "'" + strings.ReplaceAll(executable, "'", "''") + "'"
	case strings.HasSuffix(assetPath, ".ts"):
		encoded, err := json.Marshal(executable)
		if err != nil {
			return `""`
		}
		return string(encoded)
	default:
		return shellSingleQuote(executable)
	}
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
