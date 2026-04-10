package db

import (
	"os"
	"os/exec"
	"strings"
)

// detectUsername returns the current user's name via 4-tier fallback:
//  1. JARVIS_USER env var (explicit override for CI/production)
//  2. Git global config user.name (developer standard)
//  3. OS username parsed from home directory path
//  4. "unknown" (never blocks a memory save)
func detectUsername() string {
	if user := os.Getenv("JARVIS_USER"); user != "" {
		return user
	}
	if user := gitUsername(); user != "" {
		return user
	}
	if user := osUsername(); user != "" {
		return user
	}
	return "unknown"
}

func gitUsername() string {
	out, err := exec.Command("git", "config", "--global", "user.name").Output()
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(out))
	return name
}

func osUsername() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return usernameFromHome(home)
}

// usernameFromHome extracts a username from a home directory path.
// Handles Linux (/home/user), macOS (/Users/user), and Windows (C:\Users\user).
// Returns "" if the path is empty, root, or can't be parsed.
func usernameFromHome(homeDir string) string {
	if homeDir == "" {
		return ""
	}
	// Normalize both separators to handle Windows paths on any OS
	normalized := strings.ReplaceAll(homeDir, `\`, "/")
	normalized = strings.TrimRight(normalized, "/")
	if normalized == "" {
		return ""
	}
	idx := strings.LastIndex(normalized, "/")
	if idx < 0 {
		// No separator — bare name with no path component
		return normalized
	}
	base := normalized[idx+1:]
	if base == "" || base == "." {
		return ""
	}
	return base
}
