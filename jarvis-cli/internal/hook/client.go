package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// SaveStatus is the three-valued result of querying the daemon for a project's
// most recent save timestamp.
type SaveStatus int

const (
	// SaveUnreachable means the daemon could not be reached or returned an
	// unusable response (transport error, timeout, non-200, malformed body, or
	// an unparseable timestamp). Fail-safe: callers MUST NOT fire a reminder.
	SaveUnreachable SaveStatus = iota
	// SaveFound means the daemon returned a real, parseable last-save timestamp.
	SaveFound
	// SaveEmpty means the daemon was reachable but the project has never saved.
	SaveEmpty
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

// get sends a GET to path and returns the response body and true on a 200
// response. Any transport error, timeout, non-200 status, or read failure
// returns (nil, false). Like post, it is fail-safe and never surfaces an error.
func (c *DaemonClient) get(ctx context.Context, path string) ([]byte, bool) {
	tctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(tctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, false
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false // daemon down, timeout, refused
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false
	}
	return data, true
}

// LatestSaveAt queries the daemon for the most recent save timestamp of project.
//
// It is fail-safe: any transport error, timeout, non-200 status, malformed body,
// or unparseable timestamp yields SaveUnreachable so the caller never fires a
// reminder on failure. A reachable daemon with no saves yields SaveEmpty; a real
// timestamp yields SaveFound.
func (c *DaemonClient) LatestSaveAt(ctx context.Context, project string) (time.Time, SaveStatus) {
	path := "/projects/" + url.PathEscape(project) + "/last-save"
	data, ok := c.get(ctx, path)
	if !ok {
		return time.Time{}, SaveUnreachable
	}

	var body struct {
		LastSaveAt string `json:"last_save_at"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return time.Time{}, SaveUnreachable
	}
	if strings.TrimSpace(body.LastSaveAt) == "" {
		return time.Time{}, SaveEmpty
	}
	ts, err := time.Parse(time.RFC3339, body.LastSaveAt)
	if err != nil {
		return time.Time{}, SaveUnreachable
	}
	return ts, SaveFound
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
// The directory parameter is included so the daemon can derive the canonical
// project name when project is empty.
func (c *DaemonClient) PostPassiveObservation(ctx context.Context, sessionID, project, source, content, directory string) error {
	return c.post(ctx, "/observations/passive", map[string]string{
		"session_id": sessionID,
		"project":    project,
		"source":     source,
		"content":    content,
		"directory":  directory,
	})
}
