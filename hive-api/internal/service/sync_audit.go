package service

import "github.com/Thrasno/jarvis-dev/hive-api/internal/model"

const syncConflictReasonCode = "memory_conflict"

func buildSyncAuditEntries(project, userID string, pushed, conflicts, promptsPushed int) []*model.AuditEntry {
	entries := []*model.AuditEntry{
		buildSyncAuditEntry(model.AuditActionSyncPush, model.AuditOutcomeSuccess, project, userID, pushed+conflicts+promptsPushed, nil, pushed, conflicts, promptsPushed),
	}

	if conflicts > 0 {
		reason := syncConflictReasonCode
		entries = append(entries, buildSyncAuditEntry(model.AuditActionSyncConflict, model.AuditOutcomeConflict, project, userID, conflicts, &reason, pushed, conflicts, promptsPushed))
	}

	return entries
}

func buildSyncAuditEntry(action model.AuditAction, outcome model.AuditOutcome, project, userID string, entryCount int, reasonCode *string, pushed, conflicts, promptsPushed int) *model.AuditEntry {
	metadata := model.AuditMetadata{
		"pushed_count":   pushed,
		"conflict_count": conflicts,
		"prompt_count":   promptsPushed,
	}
	if reasonCode != nil {
		metadata["reason_code"] = *reasonCode
	}

	return &model.AuditEntry{
		ActorUserID: &userID,
		Project:     &project,
		Action:      action,
		Outcome:     outcome,
		EntryCount:  entryCount,
		ReasonCode:  reasonCode,
		Metadata:    model.SanitizeAuditMetadata(action, metadata),
	}
}
