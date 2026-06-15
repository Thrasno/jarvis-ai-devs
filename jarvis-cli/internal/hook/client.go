package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// DaemonClient is a fire-and-forget HTTP client for the local hive-daemon.
// All methods are non-fatal: they swallow errors and return nil so that hook
// subcommands never block or fail Claude Code.
type DaemonClient struct {
	BaseURL string
	Timeout time.Duration
}

// NewDaemonClient creates a DaemonClient using the HIVE_HTTP_PORT env var
// (default 7438) to derive the base URL.
func NewDaemonClient() *DaemonClient {
	port := os.Getenv("HIVE_HTTP_PORT")
	if port == "" {
		port = "7438"
	}
	return &DaemonClient{
		BaseURL: fmt.Sprintf("http://127.0.0.1:%s", port),
		Timeout: 5 * time.Second,
	}
}

// post marshals body to JSON and sends a POST to the given path.
// Returns nil on any network or HTTP error — all calls are fire-and-forget.
// Optionally, statusOK lists additional status codes considered successful;
// if nil, only 2xx responses are considered OK but errors are still discarded.
func (c *DaemonClient) post(ctx context.Context, path string, body any, nonFatalStatus ...int) error {
	data, err := json.Marshal(body)
	if err != nil {
		return nil // discard
	}

	tctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(tctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil // discard
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil // discard — daemon down, timeout, refused
	}
	defer resp.Body.Close()

	// Check if this status is in the explicitly non-fatal list.
	for _, s := range nonFatalStatus {
		if resp.StatusCode == s {
			return nil
		}
	}
	// All errors discarded — caller never receives an error from this client.
	return nil
}

// PostPrompt sends the user prompt to the daemon's /prompts endpoint.
func (c *DaemonClient) PostPrompt(ctx context.Context, sessionID, directory, project, content string) error {
	return c.post(ctx, "/prompts", map[string]string{
		"content":    content,
		"session_id": sessionID,
		"directory":  directory,
		"project":    project,
	})
}

// PostSessionStart notifies the daemon that a new session has started.
func (c *DaemonClient) PostSessionStart(ctx context.Context, sessionID, project, directory string) error {
	return c.post(ctx, "/sessions", map[string]string{
		"id":        sessionID,
		"project":   project,
		"directory": directory,
		"dev_id":    "",
		"client":    "hook",
	})
}

// PostSessionEnd notifies the daemon that a session has ended.
// A 404 response is treated as non-fatal (session was never created).
func (c *DaemonClient) PostSessionEnd(ctx context.Context, sessionID string) error {
	return c.post(ctx, "/sessions/"+sessionID+"/end", map[string]string{
		"summary": "",
	}, http.StatusNotFound)
}

// PostPassiveObservation records a passive observation (e.g. subagent stdout).
func (c *DaemonClient) PostPassiveObservation(ctx context.Context, sessionID, project, source, content string) error {
	return c.post(ctx, "/observations/passive", map[string]string{
		"session_id": sessionID,
		"project":    project,
		"source":     source,
		"content":    content,
	})
}
