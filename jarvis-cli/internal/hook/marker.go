package hook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// safeSessionID replaces characters that are unsafe for use in filenames with "-".
// Replaced characters: / \ : * ? " < > | and space.
func safeSessionID(id string) string {
	replacer := strings.NewReplacer(
		"/", "-",
		`\`, "-",
		":", "-",
		"*", "-",
		"?", "-",
		`"`, "-",
		"<", "-",
		">", "-",
		"|", "-",
		" ", "-",
	)
	return replacer.Replace(id)
}

// markerPath returns the absolute path of a marker file for the given session
// ID and marker name. The file base name is "<name>-<safeSessionID>.done" (e.g.
// "first-prompt-<id>.done", "memory-reminder-<id>.done"). The base directory is
// chosen using the following priority:
//
//  1. XDG_RUNTIME_DIR
//  2. TMPDIR
//  3. TEMP
//  4. TMP
//  5. os.TempDir() (covers /tmp on Unix, %TEMP% on Windows)
func markerPath(sessionID, name string) string {
	base := ""
	for _, env := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TEMP", "TMP"} {
		if v := os.Getenv(env); v != "" {
			base = v
			break
		}
	}
	if base == "" {
		base = os.TempDir()
	}
	safe := safeSessionID(sessionID)
	return filepath.Join(base, "jarvis-hive", "claude-hooks", name+"-"+safe+".done")
}

// CreateMarker creates the marker file for the given session ID.
// It creates all intermediate directories as needed.
// If the file already exists the call is a no-op (idempotent): the existing
// timestamp is PRESERVED, never rewritten. This matters because the first-prompt
// marker's timestamp is the session-age baseline for the mid-session memory
// reminder, and RunSessionStart calls CreateMarker on every SessionStart
// (including resume/clear). Rewriting it would reset the session-age clock and
// suppress the reminder on long/resumed sessions.
//
// Idempotency is achieved via an exclusive create (O_CREATE|O_EXCL): the
// timestamp is written only when this call actually created the file; if the
// file already exists the call leaves it untouched and returns nil.
func CreateMarker(sessionID string) error {
	p := markerPath(sessionID, markerFirstPrompt)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			// Marker already exists — preserve its timestamp (idempotent no-op).
			return nil
		}
		return err
	}
	defer f.Close()
	// Write a timestamp so the file is non-empty and human-readable.
	_, _ = f.WriteString(time.Now().UTC().Format(time.RFC3339) + "\n")
	return nil
}

// DeleteMarker removes the marker file for the given session ID.
// If the file does not exist the call returns nil (non-fatal).
func DeleteMarker(sessionID string) error {
	p := markerPath(sessionID, markerFirstPrompt)
	err := os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// MarkerExists reports whether the marker file for the given session ID exists.
func MarkerExists(sessionID string) bool {
	p := markerPath(sessionID, markerFirstPrompt)
	_, err := os.Stat(p)
	return err == nil
}

// CreateMarkerExclusive atomically creates the marker file for the given
// session ID using O_CREATE|O_EXCL. It returns created=true when the file did
// not exist and was created by this call, and created=false when the file
// already existed. Any other OS error is returned as err.
//
// Use this instead of the MarkerExists + CreateMarker two-step when only one
// concurrent caller should act (e.g. first-prompt detection in RunPromptSubmit).
func CreateMarkerExclusive(sessionID string) (created bool, err error) {
	p := markerPath(sessionID, markerFirstPrompt)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return false, nil
		}
		return false, err
	}
	defer f.Close()
	// Write a timestamp so the file is non-empty and human-readable.
	_, _ = f.WriteString(time.Now().UTC().Format(time.RFC3339) + "\n")
	return true, nil
}

// ReadMarkerTime reads the timestamp stored on the first line of the named
// marker for sessionID. It returns an error when the marker is missing, empty,
// or the first line is not a valid RFC3339 timestamp. Callers treat any error
// as "time unknown" and fail safe (no reminder).
func ReadMarkerTime(sessionID, name string) (time.Time, error) {
	p := markerPath(sessionID, name)
	data, err := os.ReadFile(p)
	if err != nil {
		return time.Time{}, err
	}
	line := string(data)
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = line[:idx]
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return time.Time{}, fmt.Errorf("empty marker %q", p)
	}
	return time.Parse(time.RFC3339, line)
}

// RewriteMarker creates or truncates the named marker for sessionID and writes
// the current UTC timestamp. Unlike CreateMarkerExclusive it always overwrites,
// which powers the read-then-rewrite cooldown used by the memory reminder.
func RewriteMarker(sessionID, name string) error {
	p := markerPath(sessionID, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, _ = f.WriteString(time.Now().UTC().Format(time.RFC3339) + "\n")
	return nil
}
