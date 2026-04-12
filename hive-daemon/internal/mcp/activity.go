package mcp

import (
	"fmt"
	"sync"
	"time"
)

// projectActivity tracks tool call and save activity for a single project within a session.
type projectActivity struct {
	lastToolCall time.Time
	lastSave     time.Time
	toolCalls    int
	saves        int
}

// ActivityTracker monitors per-project tool usage and generates nudges
// when the agent hasn't saved in a while despite being active.
// Thread-safe — all methods acquire the mutex.
type ActivityTracker struct {
	mu       sync.Mutex
	projects map[string]*projectActivity
	now      func() time.Time // injectable for testing
}

// NewActivityTracker creates a tracker with the real clock.
func NewActivityTracker() *ActivityTracker {
	return &ActivityTracker{
		projects: make(map[string]*projectActivity),
		now:      time.Now,
	}
}

// NewActivityTrackerWithClock creates a tracker with a custom clock (for testing).
func NewActivityTrackerWithClock(now func() time.Time) *ActivityTracker {
	return &ActivityTracker{
		projects: make(map[string]*projectActivity),
		now:      now,
	}
}

// getOrCreate returns the activity state for a project, creating if needed.
// Caller must hold a.mu.
func (a *ActivityTracker) getOrCreate(project string) *projectActivity {
	pa, ok := a.projects[project]
	if !ok {
		now := a.now()
		pa = &projectActivity{
			lastToolCall: now,
			// treat creation as "last save" to avoid instant nudge on new sessions
			lastSave: now,
		}
		a.projects[project] = pa
	}
	return pa
}

// RecordToolCall increments the tool call counter for a project.
// Called on: mem_search, mem_context, mem_get_observation.
func (a *ActivityTracker) RecordToolCall(project string) {
	if project == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	pa := a.getOrCreate(project)
	pa.toolCalls++
	pa.lastToolCall = a.now()
}

// RecordSave increments the save counter and resets the save timer.
// Called on: mem_save, mem_session_summary.
func (a *ActivityTracker) RecordSave(project string) {
	if project == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	pa := a.getOrCreate(project)
	pa.saves++
	pa.lastSave = a.now()
}

// NudgeIfNeeded returns a nudge message if the agent hasn't saved recently
// despite being active. Returns "" if no nudge is warranted.
//
// Nudge conditions (ALL must be true):
//   - time since last save > 10 minutes
//   - toolCalls > saves (more reads than writes)
//   - toolCalls >= 3 (minimum activity threshold)
func (a *ActivityTracker) NudgeIfNeeded(project string) string {
	if project == "" {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	pa, ok := a.projects[project]
	if !ok {
		return ""
	}

	sinceLastSave := a.now().Sub(pa.lastSave)
	if sinceLastSave <= 10*time.Minute {
		return ""
	}
	if pa.toolCalls <= pa.saves || pa.toolCalls < 3 {
		return ""
	}

	minutes := int(sinceLastSave.Minutes())
	return fmt.Sprintf(
		"\n\n⚠️ No mem_save calls for project %q in %d minutes. "+
			"Did you make any decisions, fix bugs, or discover something worth persisting?",
		project, minutes,
	)
}

// SessionStats returns a summary line for session summary responses.
// Includes a warning if there's high activity with no saves.
func (a *ActivityTracker) SessionStats(project string) string {
	if project == "" {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	pa, ok := a.projects[project]
	if !ok {
		return ""
	}

	stats := fmt.Sprintf("\n\nSession activity: %d tool calls, %d saves", pa.toolCalls, pa.saves)
	if pa.saves == 0 && pa.toolCalls >= 5 {
		stats += " — high activity with no saves, consider persisting important decisions"
	}
	return stats
}
