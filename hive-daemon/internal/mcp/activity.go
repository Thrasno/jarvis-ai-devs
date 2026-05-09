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

// sessionActivity tracks tool call/save/prompt activity per session_id, kept in
// a namespace separate from the per-project map so per-session and per-project
// counters don't contaminate each other (CRIT-6).
type sessionActivity struct {
	lastToolCall  time.Time
	lastSave      time.Time
	toolCalls     int
	saves         int
	currentPrompt string
}

// ActivityTracker monitors per-project tool usage and generates nudges
// when the agent hasn't saved in a while despite being active.
// Thread-safe — all methods acquire the mutex.
type ActivityTracker struct {
	mu       sync.Mutex
	projects map[string]*projectActivity
	sessions map[string]*sessionActivity
	now      func() time.Time // injectable for testing
}

// NewActivityTracker creates a tracker with the real clock.
func NewActivityTracker() *ActivityTracker {
	return &ActivityTracker{
		projects: make(map[string]*projectActivity),
		sessions: make(map[string]*sessionActivity),
		now:      time.Now,
	}
}

// NewActivityTrackerWithClock creates a tracker with a custom clock (for testing).
func NewActivityTrackerWithClock(now func() time.Time) *ActivityTracker {
	return &ActivityTracker{
		projects: make(map[string]*projectActivity),
		sessions: make(map[string]*sessionActivity),
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
// Nudge conditions (ANY must be true):
//   - message-based: (toolCalls - saves) % 5 == 0 AND (toolCalls - saves) > 0
//   - time-based: time since last save > 10 minutes AND toolCalls >= 3
//   - AND in both cases: toolCalls > saves (more reads than writes)
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

	// Common precondition: must have more reads than writes
	if pa.toolCalls <= pa.saves {
		return ""
	}

	readsSinceLastSave := pa.toolCalls - pa.saves

	// Message-based nudge: every 5 reads without saves
	if readsSinceLastSave > 0 && readsSinceLastSave%5 == 0 {
		return fmt.Sprintf(
			"\n\n⚠️ %d reads without saves in project %q. "+
				"Look for: agreement patterns (\"let's do\", \"yes, go ahead\"), "+
				"conclusions, or non-obvious discoveries worth persisting.",
			readsSinceLastSave, project,
		)
	}

	// Time-based nudge (existing behavior)
	sinceLastSave := a.now().Sub(pa.lastSave)
	if sinceLastSave > 10*time.Minute && pa.toolCalls >= 3 {
		minutes := int(sinceLastSave.Minutes())
		return fmt.Sprintf(
			"\n\n⚠️ No mem_save calls for project %q in %d minutes. "+
				"Did you make any decisions, fix bugs, or discover something worth persisting?",
			project, minutes,
		)
	}

	return ""
}

// ClearSession removes per-session tracking state for the given session ID.
// Called when a session ends so stale activity data is not retained.
// CRIT-6: this clears the dedicated sessions map (separate namespace from
// per-project counters).
func (a *ActivityTracker) ClearSession(sessionID string) {
	if sessionID == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, sessionID)
}

// getOrCreateSession returns the activity state for a session, creating if needed.
// Caller must hold a.mu.
func (a *ActivityTracker) getOrCreateSession(sessionID string) *sessionActivity {
	sa, ok := a.sessions[sessionID]
	if !ok {
		now := a.now()
		sa = &sessionActivity{
			lastToolCall: now,
			lastSave:     now,
		}
		a.sessions[sessionID] = sa
	}
	return sa
}

// RecordToolCallForSession increments the per-session tool call counter.
// Independent of per-project tracking (CRIT-6).
func (a *ActivityTracker) RecordToolCallForSession(sessionID string) {
	if sessionID == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	sa := a.getOrCreateSession(sessionID)
	sa.toolCalls++
	sa.lastToolCall = a.now()
}

// RecordSaveForSession increments the per-session save counter.
func (a *ActivityTracker) RecordSaveForSession(sessionID string) {
	if sessionID == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	sa := a.getOrCreateSession(sessionID)
	sa.saves++
	sa.lastSave = a.now()
}

// RecordPromptForSession stores the most-recent prompt content for a session.
func (a *ActivityTracker) RecordPromptForSession(sessionID, content string) {
	if sessionID == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	sa := a.getOrCreateSession(sessionID)
	sa.currentPrompt = content
}

// CurrentPromptForSession returns the most-recent prompt for a session, or "".
func (a *ActivityTracker) CurrentPromptForSession(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	sa, ok := a.sessions[sessionID]
	if !ok {
		return ""
	}
	return sa.currentPrompt
}

// SessionToolCalls returns the per-session tool call count, or 0 if unknown.
func (a *ActivityTracker) SessionToolCalls(sessionID string) int {
	if sessionID == "" {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	sa, ok := a.sessions[sessionID]
	if !ok {
		return 0
	}
	return sa.toolCalls
}

// SessionSaves returns the per-session save count, or 0 if unknown.
func (a *ActivityTracker) SessionSaves(sessionID string) int {
	if sessionID == "" {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	sa, ok := a.sessions[sessionID]
	if !ok {
		return 0
	}
	return sa.saves
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
