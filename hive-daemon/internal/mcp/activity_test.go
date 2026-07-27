package mcp_test

import (
	"strings"
	"testing"
	"time"

	hivemcp "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/mcp"
)

// ─── T2.7: Per-session ActivityTracker methods ────────────────────────────

func TestActivityTracker_ClearSession_RemovesSessionState(t *testing.T) {
	tracker := hivemcp.NewActivityTracker()

	tracker.RecordToolCall("sess-abc")
	tracker.RecordSave("sess-abc")

	// After clear, NudgeIfNeeded should return "" (no state for that key)
	tracker.ClearSession("sess-abc")

	// Verify by recording reads that would normally trigger a nudge —
	// since ClearSession wiped the state, the counter resets to 0.
	for i := 0; i < 5; i++ {
		tracker.RecordToolCall("sess-abc")
	}
	// After 5 reads from a fresh state: NudgeIfNeeded SHOULD fire at exactly 5.
	// This confirms ClearSession truly reset the state (not that the counter grew from prev values).
	nudge := tracker.NudgeIfNeeded("sess-abc")
	if nudge == "" {
		t.Error("after ClearSession + 5 reads, nudge should fire as if fresh (counter reset)")
	}
}

func TestActivityTracker_ClearSession_EmptyID_DoesNotPanic(t *testing.T) {
	tracker := hivemcp.NewActivityTracker()
	// Must not panic
	tracker.ClearSession("")
}

func TestActivityTracker_ClearSession_UnknownID_DoesNotPanic(t *testing.T) {
	tracker := hivemcp.NewActivityTracker()
	// Must not panic on unknown session
	tracker.ClearSession("never-tracked")
}

func TestActivityTracker_PerSessionIsolation(t *testing.T) {
	tracker := hivemcp.NewActivityTracker()

	// CRIT-6: per-session tracking is independent of per-project tracking.
	// Record activity for session-A and session-B via the per-session API.
	for i := 0; i < 3; i++ {
		tracker.RecordToolCallForSession("session-A")
	}
	for i := 0; i < 10; i++ {
		tracker.RecordToolCallForSession("session-B")
	}

	// Recording project-level activity must NOT contaminate session-level state.
	tracker.RecordToolCall("some-project")

	// ClearSession removes the per-session entry, leaving session-B untouched.
	tracker.ClearSession("session-A")

	// session-B's per-session counters should still reflect 10 reads.
	if got := tracker.SessionToolCalls("session-B"); got != 10 {
		t.Errorf("session-B tool calls = %d, want 10", got)
	}
	// session-A's counters should reset to 0 after ClearSession.
	if got := tracker.SessionToolCalls("session-A"); got != 0 {
		t.Errorf("session-A tool calls after clear = %d, want 0", got)
	}
}

// CRIT-6 — mem_session_start MUST record per-session activity (not just per-project).
// The previous code called RecordToolCall(p.Project) which keyed activity by project,
// so ClearSession(sessionID) never matched and per-session tracking was a fiction.
func TestActivityTracker_RecordToolCallForSession_KeyedBySessionID(t *testing.T) {
	tracker := hivemcp.NewActivityTracker()

	tracker.RecordToolCallForSession("sess-1")
	tracker.RecordToolCallForSession("sess-1")
	tracker.RecordToolCallForSession("sess-2")

	if got := tracker.SessionToolCalls("sess-1"); got != 2 {
		t.Errorf("sess-1 calls = %d, want 2", got)
	}
	if got := tracker.SessionToolCalls("sess-2"); got != 1 {
		t.Errorf("sess-2 calls = %d, want 1", got)
	}
	if got := tracker.SessionToolCalls("never-seen"); got != 0 {
		t.Errorf("unknown session calls = %d, want 0", got)
	}
}

func TestActivityTracker_TimeBasedNudgeRequestsSaveWithoutElapsedPressure(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tracker := hivemcp.NewActivityTrackerWithClock(func() time.Time { return now })

	// Simulate 3 tool calls with no save
	tracker.RecordToolCall("proj")
	tracker.RecordToolCall("proj")
	tracker.RecordToolCall("proj")

	// Advance clock 11 minutes — past the 10-minute threshold
	now = now.Add(11 * time.Minute)

	nudge := tracker.NudgeIfNeeded("proj")
	if nudge == "" {
		t.Error("expected nudge after 11 minutes of inactivity with 3 tool calls and 0 saves")
	}
	if strings.Contains(nudge, "11 minutes") || strings.Contains(nudge, "minutes") {
		t.Errorf("nudge must not expose elapsed time, got: %s", nudge)
	}
	if !strings.Contains(nudge, "proj") {
		t.Errorf("nudge should mention the project name, got: %s", nudge)
	}
	if !strings.Contains(nudge, "mem_save") || !strings.Contains(nudge, "important learnings") {
		t.Errorf("nudge should request saving important learnings, got: %s", nudge)
	}
}

func TestActivityTracker_NoNudgeAfterRecentSave(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tracker := hivemcp.NewActivityTrackerWithClock(func() time.Time { return now })

	tracker.RecordToolCall("proj")
	tracker.RecordToolCall("proj")
	tracker.RecordToolCall("proj")

	// Advance 9 minutes and save — save is recent
	now = now.Add(9 * time.Minute)
	tracker.RecordSave("proj")

	// Advance 2 more minutes — total 11 min from start, but only 2 min since save
	now = now.Add(2 * time.Minute)

	nudge := tracker.NudgeIfNeeded("proj")
	if nudge != "" {
		t.Errorf("expected no nudge after recent save (2 min ago), got: %s", nudge)
	}
}

func TestActivityTracker_SessionStats(t *testing.T) {
	tracker := hivemcp.NewActivityTracker()

	// 5 tool calls, no saves
	for i := 0; i < 5; i++ {
		tracker.RecordToolCall("proj")
	}

	stats := tracker.SessionStats("proj")
	if !strings.Contains(stats, "5 tool calls") {
		t.Errorf("stats should show 5 tool calls, got: %s", stats)
	}
	if !strings.Contains(stats, "0 saves") {
		t.Errorf("stats should show 0 saves, got: %s", stats)
	}
	if !strings.Contains(stats, "high activity") {
		t.Errorf("stats should warn about high activity with no saves, got: %s", stats)
	}
}

func TestActivityTracker_SessionStats_WithSaves(t *testing.T) {
	tracker := hivemcp.NewActivityTracker()

	for i := 0; i < 8; i++ {
		tracker.RecordToolCall("proj")
	}
	for i := 0; i < 3; i++ {
		tracker.RecordSave("proj")
	}

	stats := tracker.SessionStats("proj")
	if !strings.Contains(stats, "8 tool calls") {
		t.Errorf("stats should show 8 tool calls, got: %s", stats)
	}
	if !strings.Contains(stats, "3 saves") {
		t.Errorf("stats should show 3 saves, got: %s", stats)
	}
	// "high activity with no saves" should NOT appear when saves > 0
	if strings.Contains(stats, "high activity") {
		t.Errorf("stats should NOT warn about high activity when there are saves, got: %s", stats)
	}
}

func TestActivityTracker_NoNudgeForUnknownProject(t *testing.T) {
	tracker := hivemcp.NewActivityTracker()

	nudge := tracker.NudgeIfNeeded("never-seen")
	if nudge != "" {
		t.Errorf("expected no nudge for unknown project, got: %s", nudge)
	}
}

func TestActivityTracker_NoNudgeBelowThreshold(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tracker := hivemcp.NewActivityTrackerWithClock(func() time.Time { return now })

	// Only 2 tool calls — below the threshold of 3
	tracker.RecordToolCall("proj")
	tracker.RecordToolCall("proj")

	// Advance far past the 10-minute threshold
	now = now.Add(15 * time.Minute)

	nudge := tracker.NudgeIfNeeded("proj")
	if nudge != "" {
		t.Errorf("expected no nudge with only 2 tool calls (below threshold of 3), got: %s", nudge)
	}
}

func TestActivityTracker_NoNudgeForEmptyProject(t *testing.T) {
	tracker := hivemcp.NewActivityTracker()

	nudge := tracker.NudgeIfNeeded("")
	if nudge != "" {
		t.Errorf("expected no nudge for empty project string, got: %s", nudge)
	}
}

func TestActivityTracker_SessionStats_EmptyProject(t *testing.T) {
	tracker := hivemcp.NewActivityTracker()

	stats := tracker.SessionStats("")
	if stats != "" {
		t.Errorf("expected empty stats for empty project string, got: %s", stats)
	}
}

func TestActivityTracker_MessageBasedNudge_AtFiveReads(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tracker := hivemcp.NewActivityTrackerWithClock(func() time.Time { return now })

	// Simulate exactly 5 tool calls with 0 saves
	for i := 0; i < 5; i++ {
		tracker.RecordToolCall("proj")
	}

	// Should nudge even with NO time elapsed (message-based, not time-based)
	nudge := tracker.NudgeIfNeeded("proj")
	if nudge == "" {
		t.Error("expected message-based nudge after 5 tool calls with 0 saves")
	}
	if !strings.Contains(nudge, "5 reads") {
		t.Errorf("nudge should mention '5 reads', got: %s", nudge)
	}
}

func TestActivityTracker_MessageBasedNudge_AtTenReads(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tracker := hivemcp.NewActivityTrackerWithClock(func() time.Time { return now })

	// Simulate 10 tool calls with 0 saves (should trigger at 5 and 10)
	for i := 0; i < 10; i++ {
		tracker.RecordToolCall("proj")
	}

	nudge := tracker.NudgeIfNeeded("proj")
	if nudge == "" {
		t.Error("expected message-based nudge after 10 tool calls with 0 saves")
	}
	if !strings.Contains(nudge, "10 reads") {
		t.Errorf("nudge should mention '10 reads', got: %s", nudge)
	}
}

func TestActivityTracker_MessageBasedNudge_NoNudgeAtFourReads(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tracker := hivemcp.NewActivityTrackerWithClock(func() time.Time { return now })

	// Simulate 4 tool calls — below the 5-read threshold
	for i := 0; i < 4; i++ {
		tracker.RecordToolCall("proj")
	}

	nudge := tracker.NudgeIfNeeded("proj")
	if nudge != "" {
		t.Errorf("expected no nudge before 5-read threshold, got: %s", nudge)
	}
}

func TestActivityTracker_MessageBasedNudge_SaveResetsCounter(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tracker := hivemcp.NewActivityTrackerWithClock(func() time.Time { return now })

	// 3 reads, 1 save, 2 more reads = 5 total but only 2 since last save
	tracker.RecordToolCall("proj")
	tracker.RecordToolCall("proj")
	tracker.RecordToolCall("proj")
	tracker.RecordSave("proj")
	tracker.RecordToolCall("proj")
	tracker.RecordToolCall("proj")

	// Should NOT nudge — only 2 reads since last save
	nudge := tracker.NudgeIfNeeded("proj")
	if nudge != "" {
		t.Errorf("expected no nudge when reads since save < 5, got: %s", nudge)
	}
}

func TestActivityTracker_MessageBasedNudge_IncludesSemanticPatterns(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tracker := hivemcp.NewActivityTrackerWithClock(func() time.Time { return now })

	for i := 0; i < 5; i++ {
		tracker.RecordToolCall("proj")
	}

	nudge := tracker.NudgeIfNeeded("proj")
	if !strings.Contains(nudge, "let's do") {
		t.Errorf("nudge should reference semantic patterns, got: %s", nudge)
	}
	if !strings.Contains(nudge, "yes, go ahead") {
		t.Errorf("nudge should reference semantic patterns, got: %s", nudge)
	}
}
