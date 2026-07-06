package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/stretchr/testify/require"
)

func TestOpen_ProjectBlocksSchemaExists(t *testing.T) {
	d := openTestDB(t)

	for _, tt := range []struct {
		kind string
		name string
	}{
		{kind: "table", name: "project_blocks"},
		{kind: "index", name: "idx_project_blocks_canonical"},
		{kind: "index", name: "idx_project_blocks_pending_ack"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var name string
			err := d.sqlDB.QueryRow(
				"SELECT name FROM sqlite_master WHERE type = ? AND name = ?", tt.kind, tt.name,
			).Scan(&name)
			require.NoErrorf(t, err, "%s %s should exist", tt.kind, tt.name)
		})
	}
}

func TestDB_RecordProjectBlockCanonicalizesAndPersistsAck(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	blockedAt := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	ackAt := blockedAt.Add(time.Minute)

	block, err := d.RecordProjectBlock(ctx, ProjectBlockCommand{
		CommandID:           "cmd-1",
		AckToken:            "ack-token-1",
		Project:             "  My Project  ",
		CanonicalProjectKey: "my-project",
		BlockedAt:           blockedAt,
	})
	require.NoError(t, err)
	require.Equal(t, "my-project", block.CanonicalProjectKey)
	require.True(t, block.AckPending)

	loaded, err := d.GetProjectBlock(ctx, "MY PROJECT")
	require.NoError(t, err)
	require.Equal(t, "cmd-1", loaded.CommandID)
	require.Equal(t, "ack-token-1", loaded.AckToken)
	require.Equal(t, "my-project", loaded.CanonicalProjectKey)
	require.True(t, loaded.AckPending)

	recorded, err := d.RecordProjectBlockAck(ctx, ProjectBlockAck{
		CommandID:           "cmd-1",
		CanonicalProjectKey: "MY PROJECT",
		AckToken:            "ack-token-1",
		Status:              ProjectBlockAckApplied,
		Warning:             "archived locally",
		AppliedAt:           ackAt,
	})
	require.NoError(t, err)
	require.Equal(t, "my-project", recorded.CanonicalProjectKey)
	require.False(t, recorded.AppliedAt.IsZero())

	acked, err := d.GetProjectBlock(ctx, "my-project")
	require.NoError(t, err)
	require.False(t, acked.AckPending)
	require.Equal(t, ProjectBlockAckApplied, acked.AckStatus)
	require.Equal(t, "archived locally", acked.AckWarning)
	require.Equal(t, ackAt, acked.AckAppliedAt)
}

func TestDB_ListPendingProjectBlockAcks(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	blockedAt := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	ackAt := blockedAt.Add(time.Minute)

	_, err := d.RecordProjectBlock(ctx, ProjectBlockCommand{CommandID: "cmd-pending", AckToken: "ack-token-pending", Project: "pending", CanonicalProjectKey: "pending", BlockedAt: blockedAt})
	require.NoError(t, err)
	_, err = d.RecordProjectBlock(ctx, ProjectBlockCommand{CommandID: "cmd-acked", AckToken: "ack-token-acked", Project: "acked", CanonicalProjectKey: "acked", BlockedAt: blockedAt})
	require.NoError(t, err)
	_, err = d.RecordProjectBlockAck(ctx, ProjectBlockAck{CommandID: "cmd-acked", CanonicalProjectKey: "acked", AckToken: "ack-token-acked", Status: ProjectBlockAckApplied, AppliedAt: ackAt})
	require.NoError(t, err)

	pending, err := d.ListPendingProjectBlockAcks(ctx, 10)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "cmd-pending", pending[0].CommandID)
	require.Equal(t, "ack-token-pending", pending[0].AckToken)
	require.Equal(t, "pending", pending[0].CanonicalProjectKey)
	require.Equal(t, ProjectBlockAckFailed, pending[0].Status)
	require.Contains(t, pending[0].Warning, "missing durable status")
	require.False(t, pending[0].AppliedAt.IsZero())
}

func TestDB_ListPendingProjectBlockAcksPreservesFailedStatusAndWarning(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	blockedAt := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)

	_, err := d.RecordProjectBlock(ctx, ProjectBlockCommand{CommandID: "cmd-failed", AckToken: "ack-token-failed", Project: "blocked", CanonicalProjectKey: "blocked", BlockedAt: blockedAt})
	require.NoError(t, err)
	require.NoError(t, d.RecordPendingProjectBlockAck(ctx, ProjectBlockAck{CommandID: "cmd-failed", CanonicalProjectKey: "blocked", AckToken: "ack-token-failed", Status: ProjectBlockAckFailed, Warning: "archive failed", AppliedAt: blockedAt.Add(time.Minute)}))

	pending, err := d.ListPendingProjectBlockAcks(ctx, 10)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "cmd-failed", pending[0].CommandID)
	require.Equal(t, "ack-token-failed", pending[0].AckToken)
	require.Equal(t, ProjectBlockAckFailed, pending[0].Status)
	require.Equal(t, "archive failed", pending[0].Warning)
	require.Equal(t, blockedAt.Add(time.Minute), pending[0].AppliedAt)
}

func TestDB_ListPendingProjectBlockAcksDoesNotDefaultBlankStatusToApplied(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	blockedAt := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)

	_, err := d.RecordProjectBlock(ctx, ProjectBlockCommand{CommandID: "cmd-blank", AckToken: "ack-token-blank", Project: "blocked", CanonicalProjectKey: "blocked", BlockedAt: blockedAt})
	require.NoError(t, err)

	pending, err := d.ListPendingProjectBlockAcks(ctx, 10)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "cmd-blank", pending[0].CommandID)
	require.Equal(t, ProjectBlockAckFailed, pending[0].Status)
	require.Contains(t, pending[0].Warning, "missing durable status")
}

func TestDB_BlockedProjectWriteGuards(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	_, err := d.EnsureManualSaveSession("blocked")
	require.NoError(t, err)
	memoryID, err := d.SaveMemory(&models.Memory{Project: "blocked", Title: "existing", Content: "memory", SessionID: "manual-save-blocked"})
	require.NoError(t, err)
	require.NoError(t, d.DeleteMemory(memoryID, "tester", "setup deleted before block"))
	require.NoError(t, d.CreateSession("session-to-end", "blocked", "/tmp/blocked", "dev", "client"))

	_, err = d.RecordProjectBlock(ctx, ProjectBlockCommand{CommandID: "cmd-1", AckToken: "ack-token-1", Project: "blocked", CanonicalProjectKey: "blocked", BlockedAt: time.Now().UTC()})
	require.NoError(t, err)

	_, err = d.SaveMemory(&models.Memory{Project: "blocked", Title: "blocked", Content: "must not save", SessionID: "manual-save-blocked"})
	require.ErrorIs(t, err, ErrProjectBlocked)

	_, err = d.SavePrompt(ctx, "blocked", "must not save")
	require.ErrorIs(t, err, ErrProjectBlocked)

	err = d.CreateSession("session-blocked", "blocked", "/tmp/blocked", "dev", "client")
	require.ErrorIs(t, err, ErrProjectBlocked)

	_, err = d.EnsureManualSaveSession("blocked")
	require.ErrorIs(t, err, ErrProjectBlocked)

	err = d.DeleteMemory(memoryID, "tester", "blocked delete")
	require.ErrorIs(t, err, ErrProjectBlocked)

	err = d.RestoreMemory(memoryID, "tester")
	require.ErrorIs(t, err, ErrProjectBlocked)

	err = d.EndSession("session-to-end", "blocked end")
	require.ErrorIs(t, err, ErrProjectBlocked)
}

func TestDB_BlockedProjectReadFilters(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	sessionID, err := d.EnsureManualSaveSession("blocked")
	require.NoError(t, err)
	memoryID, err := d.SaveMemory(&models.Memory{Project: "blocked", Title: "secret token", Content: "quarantined content", SessionID: sessionID})
	require.NoError(t, err)
	_, err = d.SavePromptForSession(ctx, "blocked", sessionID, "hidden prompt")
	require.NoError(t, err)
	openSessionID, err := d.EnsureManualSaveSession("open")
	require.NoError(t, err)
	_, err = d.SaveMemory(&models.Memory{Project: "open", Title: "visible token", Content: "visible content", SessionID: openSessionID})
	require.NoError(t, err)

	_, err = d.RecordProjectBlock(ctx, ProjectBlockCommand{CommandID: "cmd-1", AckToken: "ack-token-1", Project: "blocked", CanonicalProjectKey: "blocked", BlockedAt: time.Now().UTC()})
	require.NoError(t, err)

	_, err = d.GetMemory(memoryID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "memory not found")

	memories, err := d.ListMemories("blocked", 10)
	require.NoError(t, err)
	require.Empty(t, memories)

	searchAll, err := d.Search("token", "", "", 10)
	require.NoError(t, err)
	require.Len(t, searchAll, 1)
	require.Equal(t, "open", searchAll[0].Project)

	searchBlocked, err := d.Search("secret", "blocked", "", 10)
	require.NoError(t, err)
	require.Empty(t, searchBlocked)

	prompt, err := d.LatestPromptForSession(ctx, "blocked", sessionID)
	require.NoError(t, err)
	require.Nil(t, prompt)

	prompts, err := d.ListRecentPrompts(ctx, "blocked", 10)
	require.NoError(t, err)
	require.Empty(t, prompts)

	_, err = d.GetSession(sessionID)
	require.ErrorIs(t, err, ErrSessionNotFound)

	sessions, err := d.ListSessions("blocked", 10)
	require.NoError(t, err)
	require.Empty(t, sessions)

	governanceMemories, err := d.ListGovernanceMemories(ctx, GovernanceMemoryFilter{Project: "blocked", IncludeDeleted: true, Limit: 10})
	require.NoError(t, err)
	require.Empty(t, governanceMemories)

	timeline, err := d.ListGovernanceMemories(ctx, GovernanceMemoryFilter{Project: "blocked", Categories: []string{"discovery"}, OrderAsc: true, Limit: 10})
	require.NoError(t, err)
	require.Empty(t, timeline)

	_, err = d.GetGovernanceMemoryByID(ctx, memoryID)
	require.ErrorIs(t, err, ErrGovernanceMemoryNotFound)
}

func TestDB_QuarantineProjectArchiveDoesNotHardPurge(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	_, err := d.EnsureManualSaveSession("blocked")
	require.NoError(t, err)
	_, err = d.SavePrompt(ctx, "blocked", "existing prompt")
	require.NoError(t, err)
	_, err = d.SaveMemory(&models.Memory{Project: "blocked", Title: "existing", Content: "memory", SessionID: "manual-save-blocked"})
	require.NoError(t, err)
	_, err = d.RecordProjectBlock(ctx, ProjectBlockCommand{CommandID: "cmd-1", AckToken: "ack-token-1", Project: "blocked", CanonicalProjectKey: "blocked", BlockedAt: time.Now().UTC()})
	require.NoError(t, err)

	result, err := d.QuarantineBlockedProject(ctx, "blocked", "daemon", "project blocked by Hive API", time.Now().UTC())
	require.NoError(t, err)
	require.True(t, result.Mutated)
	require.Equal(t, "blocked", result.Project)

	var memories, prompts, sessions int
	require.NoError(t, d.sqlDB.QueryRow(`SELECT COUNT(*) FROM memories WHERE project = 'blocked'`).Scan(&memories))
	require.NoError(t, d.sqlDB.QueryRow(`SELECT COUNT(*) FROM user_prompts WHERE project = 'blocked'`).Scan(&prompts))
	require.NoError(t, d.sqlDB.QueryRow(`SELECT COUNT(*) FROM sessions WHERE project = 'blocked'`).Scan(&sessions))
	require.Greater(t, memories, 0)
	require.Greater(t, prompts, 0)
	require.Greater(t, sessions, 0)

	_, err = d.ArchiveGovernanceProject(ctx, "blocked", "daemon", "again", time.Now().UTC())
	require.NoError(t, err)

	_, err = d.SaveMemory(&models.Memory{Project: "other", Title: "ok", Content: "ok", SessionID: "manual-save-other"})
	require.False(t, errors.Is(err, ErrProjectBlocked), "unblocked projects must not trip block guard")
}

func TestDB_QuarantineBlockedProjectRecordsUnsafeRootWarning(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	require.NoError(t, d.CreateSession("unsafe-session", "blocked", "/tmp", "dev", "client"))
	_, err := d.RecordProjectBlock(ctx, ProjectBlockCommand{CommandID: "cmd-1", AckToken: "ack-token-1", Project: "blocked", CanonicalProjectKey: "blocked", BlockedAt: time.Now().UTC()})
	require.NoError(t, err)

	result, err := d.QuarantineBlockedProject(ctx, "blocked", "daemon", "project blocked", time.Now().UTC())
	require.NoError(t, err)
	require.NotEmpty(t, result.Warning)

	warnings, err := d.ListHiveWarnings(HiveWarningFilter{ResolutionState: "active"})
	require.NoError(t, err)
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0].Message, "unsafe broad directory")
}
