package main

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveui"
)

const hiveDaemonDefaultURL = "http://127.0.0.1:7438"

var hiveDaemonURL string

var hiveCmd = &cobra.Command{
	Use:   "hive",
	Short: "Browse Hive governance TUI (live daemon data)",
	RunE: func(cmd *cobra.Command, args []string) error {
		baseURL := hiveDaemonURL
		if baseURL == "" {
			baseURL = resolveHiveDaemonURL()
		}
		return hiveui.RunHiveTUI(cmd.Context(), baseURL)
	},
}

func init() {
	hiveCmd.Flags().StringVar(&hiveDaemonURL, "daemon-url", "", "hive-daemon base URL (overrides HIVE_DAEMON_URL env var)")
}

// resolveHiveDaemonURL returns the hive-daemon base URL using the same env
// resolution as hiveclient.NewFromEnv: HIVE_DAEMON_URL first, then
// HIVE_HTTP_PORT loopback, then the hardcoded default.
func resolveHiveDaemonURL() string {
	if u := strings.TrimSpace(os.Getenv("HIVE_DAEMON_URL")); u != "" {
		return u
	}
	if port := strings.TrimSpace(os.Getenv("HIVE_HTTP_PORT")); port != "" {
		return "http://127.0.0.1:" + port
	}
	return hiveDaemonDefaultURL
}
