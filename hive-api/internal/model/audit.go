package model

import "time"

const (
	DefaultAuditLimit = 20
	MaxAuditLimit     = 100
)

type AuditAction string

const (
	AuditActionSyncPush        AuditAction = "sync_push"
	AuditActionSyncConflict    AuditAction = "sync_conflict"
	AuditActionUserLevelChange AuditAction = "user_level_change"
	AuditActionUserDeactivate  AuditAction = "user_deactivate"
)

type AuditOutcome string

const (
	AuditOutcomeSuccess  AuditOutcome = "success"
	AuditOutcomeFailure  AuditOutcome = "failure"
	AuditOutcomeConflict AuditOutcome = "conflict"
)

type AuditMetadata map[string]any

type AuditEntry struct {
	ID          string        `json:"id"`
	OccurredAt  time.Time     `json:"occurred_at"`
	ActorUserID *string       `json:"actor_user_id,omitempty"`
	Project     *string       `json:"project,omitempty"`
	Action      AuditAction   `json:"action"`
	Outcome     AuditOutcome  `json:"outcome"`
	EntryCount  int           `json:"entry_count"`
	ReasonCode  *string       `json:"reason_code,omitempty"`
	Metadata    AuditMetadata `json:"metadata"`
}

type AuditFilter struct {
	Project     *string
	ActorUserID *string
	Action      *AuditAction
	Outcome     *AuditOutcome
	Since       *time.Time
	Until       *time.Time
	Limit       int
	Offset      int
}

type AuditListResponse struct {
	AuditLogs []*AuditEntry `json:"audit_logs"`
	Total     int64         `json:"total"`
	Limit     int           `json:"limit"`
	Offset    int           `json:"offset"`
}

func (f AuditFilter) Normalize() AuditFilter {
	if f.Limit <= 0 {
		f.Limit = DefaultAuditLimit
	}
	if f.Limit > MaxAuditLimit {
		f.Limit = MaxAuditLimit
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	return f
}

func NewAuditListResponse(entries []*AuditEntry, total int64, filter AuditFilter) AuditListResponse {
	filter = filter.Normalize()
	if entries == nil {
		entries = []*AuditEntry{}
	}
	return AuditListResponse{
		AuditLogs: entries,
		Total:     total,
		Limit:     filter.Limit,
		Offset:    filter.Offset,
	}
}

func SanitizeAuditMetadata(action AuditAction, metadata map[string]any) AuditMetadata {
	allowed := auditMetadataAllowlist(action)
	clean := AuditMetadata{}
	for key, value := range metadata {
		if allowed[key] {
			clean[key] = value
		}
	}
	return clean
}

func auditMetadataAllowlist(action AuditAction) map[string]bool {
	switch action {
	case AuditActionUserLevelChange:
		return map[string]bool{
			"target_username": true,
			"target_user_id":  true,
			"old_level":       true,
			"new_level":       true,
			"actor_username":  true,
		}
	case AuditActionUserDeactivate:
		return map[string]bool{
			"target_username": true,
			"target_user_id":  true,
			"actor_username":  true,
			"reason":          true,
		}
	case AuditActionSyncPush:
		return map[string]bool{
			"pushed_count":   true,
			"conflict_count": true,
			"prompt_count":   true,
		}
	case AuditActionSyncConflict:
		return map[string]bool{
			"pushed_count":   true,
			"conflict_count": true,
			"prompt_count":   true,
			"reason_code":    true,
		}
	default:
		return map[string]bool{}
	}
}
