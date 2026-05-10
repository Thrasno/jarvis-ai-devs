package service

import "github.com/Thrasno/jarvis-dev/hive-api/internal/model"

func buildUserLevelChangeAudit(actor model.AdminActor, target *model.User, newLevel model.UserLevel) *model.AuditEntry {
	actorID := actor.UserID
	return &model.AuditEntry{
		ActorUserID: &actorID,
		Action:      model.AuditActionUserLevelChange,
		Outcome:     model.AuditOutcomeSuccess,
		Metadata: model.SanitizeAuditMetadata(model.AuditActionUserLevelChange, model.AuditMetadata{
			"target_username": target.Username,
			"target_user_id":  target.ID,
			"old_level":       string(target.Level),
			"new_level":       string(newLevel),
			"actor_username":  actor.Username,
		}),
	}
}

func buildUserDeactivateAudit(actor model.AdminActor, target *model.User) *model.AuditEntry {
	actorID := actor.UserID
	return &model.AuditEntry{
		ActorUserID: &actorID,
		Action:      model.AuditActionUserDeactivate,
		Outcome:     model.AuditOutcomeSuccess,
		Metadata: model.SanitizeAuditMetadata(model.AuditActionUserDeactivate, model.AuditMetadata{
			"target_username": target.Username,
			"target_user_id":  target.ID,
			"actor_username":  actor.Username,
		}),
	}
}
