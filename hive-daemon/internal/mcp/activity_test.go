package mcp_test

import (
	"strings"
	"testing"
	"time"

	hivemcp "github.com/Thrasno/jarvis-dev/hive-daemon/internal/mcp"
)

func TestActivityTracker_NudgeAfterInactivity(t *testing.T) {
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
	if !strings.Contains(nudge, "11 minutes") {
		t.Errorf("nudge should mention duration, got: %s", nudge)
	}
	if !strings.Contains(nudge, "proj") {
		t.Errorf("nudge should mention the project name, got: %s", nudge)
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
