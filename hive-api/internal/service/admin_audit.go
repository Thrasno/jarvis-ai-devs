package service

import (
	"errors"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
)

const (
	auditReasonMaxAdminsReached   = "max_admins_reached"
	auditReasonInsufficientAdmins = "insufficient_admins"
	auditReasonSelfAdminMutation  = "self_admin_mutation"
	auditReasonTargetNotFound     = "target_not_found"
	auditReasonTargetConflict     = "target_conflict"
	auditReasonMutationFailed     = "mutation_failed"
)

func buildUserCreateAudit(actor model.AdminActor, target *model.User) *model.AuditEntry {
	actorID := actor.UserID
	return &model.AuditEntry{
		ActorUserID: &actorID,
		Action:      model.AuditActionUserCreate,
		Outcome:     model.AuditOutcomeSuccess,
		Metadata: model.SanitizeAuditMetadata(model.AuditActionUserCreate, model.AuditMetadata{
			"target_username": target.Username,
			"target_user_id":  target.ID,
			"target_level":    string(target.Level),
			"actor_username":  actor.Username,
		}),
	}
}

func buildUserCreateFailureAudit(actor model.AdminActor, req model.CreateUserRequest, err error) *model.AuditEntry {
	actorID := actor.UserID
	reason := adminMutationFailureReason(err)
	return &model.AuditEntry{
		ActorUserID: &actorID,
		Action:      model.AuditActionUserCreate,
		Outcome:     model.AuditOutcomeFailure,
		ReasonCode:  &reason,
		Metadata: model.SanitizeAuditMetadata(model.AuditActionUserCreate, model.AuditMetadata{
			"target_username": req.Username,
			"target_level":    string(req.Level),
			"actor_username":  actor.Username,
		}),
	}
}

func buildUserPasswordResetAudit(actor model.AdminActor, target *model.User) *model.AuditEntry {
	actorID := actor.UserID
	return &model.AuditEntry{
		ActorUserID: &actorID,
		Action:      model.AuditActionUserPasswordReset,
		Outcome:     model.AuditOutcomeSuccess,
		Metadata: model.SanitizeAuditMetadata(model.AuditActionUserPasswordReset, model.AuditMetadata{
			"target_username": target.Username,
			"target_user_id":  target.ID,
			"actor_username":  actor.Username,
		}),
	}
}

func buildUserPasswordResetFailureAudit(actor model.AdminActor, username string, target *model.User, err error) *model.AuditEntry {
	actorID := actor.UserID
	reason := adminMutationFailureReason(err)
	metadata := model.AuditMetadata{
		"target_username": username,
		"actor_username":  actor.Username,
	}
	if target != nil {
		metadata["target_user_id"] = target.ID
	}
	return &model.AuditEntry{
		ActorUserID: &actorID,
		Action:      model.AuditActionUserPasswordReset,
		Outcome:     model.AuditOutcomeFailure,
		ReasonCode:  &reason,
		Metadata:    model.SanitizeAuditMetadata(model.AuditActionUserPasswordReset, metadata),
	}
}

func buildUserActivateAudit(actor model.AdminActor, target *model.User) *model.AuditEntry {
	actorID := actor.UserID
	return &model.AuditEntry{
		ActorUserID: &actorID,
		Action:      model.AuditActionUserActivate,
		Outcome:     model.AuditOutcomeSuccess,
		Metadata: model.SanitizeAuditMetadata(model.AuditActionUserActivate, model.AuditMetadata{
			"target_username": target.Username,
			"target_user_id":  target.ID,
			"actor_username":  actor.Username,
		}),
	}
}

func buildUserActivateFailureAudit(actor model.AdminActor, username string, target *model.User, err error) *model.AuditEntry {
	actorID := actor.UserID
	reason := adminMutationFailureReason(err)
	metadata := model.AuditMetadata{
		"target_username": username,
		"actor_username":  actor.Username,
	}
	if target != nil {
		metadata["target_user_id"] = target.ID
	}
	return &model.AuditEntry{
		ActorUserID: &actorID,
		Action:      model.AuditActionUserActivate,
		Outcome:     model.AuditOutcomeFailure,
		ReasonCode:  &reason,
		Metadata:    model.SanitizeAuditMetadata(model.AuditActionUserActivate, metadata),
	}
}

func adminMutationFailureReason(err error) string {
	switch {
	case errors.Is(err, ErrMaxAdminsReached):
		return auditReasonMaxAdminsReached
	case errors.Is(err, ErrInsufficientAdmins):
		return auditReasonInsufficientAdmins
	case errors.Is(err, ErrSelfAdminMutation):
		return auditReasonSelfAdminMutation
	case errors.Is(err, repository.ErrNotFound):
		return auditReasonTargetNotFound
	case errors.Is(err, repository.ErrConflict):
		return auditReasonTargetConflict
	default:
		return auditReasonMutationFailed
	}
}

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

func buildUserLevelChangeFailureAudit(actor model.AdminActor, username string, target *model.User, newLevel model.UserLevel, err error) *model.AuditEntry {
	actorID := actor.UserID
	reason := adminMutationFailureReason(err)
	metadata := model.AuditMetadata{
		"target_username": username,
		"new_level":       string(newLevel),
		"actor_username":  actor.Username,
	}
	if target != nil {
		metadata["target_username"] = target.Username
		metadata["target_user_id"] = target.ID
		metadata["old_level"] = string(target.Level)
	}
	return &model.AuditEntry{
		ActorUserID: &actorID,
		Action:      model.AuditActionUserLevelChange,
		Outcome:     model.AuditOutcomeFailure,
		ReasonCode:  &reason,
		Metadata:    model.SanitizeAuditMetadata(model.AuditActionUserLevelChange, metadata),
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

func buildUserDeactivateFailureAudit(actor model.AdminActor, username string, target *model.User, err error) *model.AuditEntry {
	actorID := actor.UserID
	reason := adminMutationFailureReason(err)
	metadata := model.AuditMetadata{
		"target_username": username,
		"actor_username":  actor.Username,
	}
	if target != nil {
		metadata["target_username"] = target.Username
		metadata["target_user_id"] = target.ID
	}
	return &model.AuditEntry{
		ActorUserID: &actorID,
		Action:      model.AuditActionUserDeactivate,
		Outcome:     model.AuditOutcomeFailure,
		ReasonCode:  &reason,
		Metadata:    model.SanitizeAuditMetadata(model.AuditActionUserDeactivate, metadata),
	}
}
