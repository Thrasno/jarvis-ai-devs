package tui

import (
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/apiclient"
)

func normalizeHiveCloudEmail(input string) (string, error) {
	email := strings.TrimSpace(input)
	if email == "" {
		return "", nil
	}
	return apiclient.NormalizeLoginEmail(email)
}
