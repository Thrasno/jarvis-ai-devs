package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/apiclient"
	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/config"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Re-authenticate with Hive Cloud",
	RunE: func(cmd *cobra.Command, args []string) error {
		scanner := bufio.NewScanner(os.Stdin)

		// Load current config to get the API URL.
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		// Prompt for credentials.
		fmt.Print("Email: ")
		email := ""
		if scanner.Scan() {
			email = strings.TrimSpace(scanner.Text())
		}
		if email == "" {
			return fmt.Errorf("email cannot be empty")
		}

		fmt.Print("Password: ")
		password := ""
		if scanner.Scan() {
			password = strings.TrimSpace(scanner.Text())
		}
		if password == "" {
			return fmt.Errorf("password cannot be empty")
		}

		// Authenticate.
		fmt.Printf("Authenticating as %s ...\n", email)
		c := apiclient.New(cfg.APIURL)
		resp, loginErr := c.Login(email, password)
		if loginErr != nil {
			return fmt.Errorf("authentication failed: %w", loginErr)
		}

		// Update config.
		cfg.Email = resp.User.Email
		if saveErr := config.Save(cfg); saveErr != nil {
			return fmt.Errorf("save config: %w", saveErr)
		}

		// Write sync.json with new credentials.
		// token is intentionally excluded — hive-daemon uses DisallowUnknownFields().
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return fmt.Errorf("get home dir: %w", homeErr)
		}
		syncJSON := fmt.Sprintf(`{"api_url":%q,"email":%q,"password":%q}`,
			cfg.APIURL, email, password)
		syncPath := filepath.Join(home, ".jarvis", "sync.json")
		if writeErr := os.WriteFile(syncPath, []byte(syncJSON), 0600); writeErr != nil {
			return fmt.Errorf("write sync.json: %w", writeErr)
		}

		fmt.Printf("Authenticated as %s. Credentials saved.\n", resp.User.Email)
		return nil
	},
}
