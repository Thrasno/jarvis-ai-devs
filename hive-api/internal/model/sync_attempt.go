package model

import "time"

const MaxSyncAttemptBatchSize = 100

type SyncAttemptOutcome string

const (
	SyncAttemptOutcomeSuccess SyncAttemptOutcome = "success"
	SyncAttemptOutcomeFailure SyncAttemptOutcome = "failure"

	SyncAttemptPortalUserSourceAuthSubject = "auth_subject"
	SyncAttemptPortalUserSourceAdminDevID  = "admin_dev_id"
)

type SyncAttemptActor struct {
	UserID string
	Level  UserLevel
}

type SyncAttemptIngestRequest struct {
	Attempts []SyncAttemptPayload `json:"attempts" binding:"max=100,dive"`
}

type SyncAttemptPayload struct {
	AttemptID    string             `json:"attempt_id" binding:"required"`
	DevID        string             `json:"dev_id"`
	Project      string             `json:"project" binding:"required"`
	Client       string             `json:"client"`
	DaemonID     string             `json:"daemon_id"`
	StartedAt    time.Time          `json:"started_at" binding:"required"`
	EndedAt      *time.Time         `json:"ended_at"`
	Outcome      SyncAttemptOutcome `json:"outcome" binding:"required"`
	HTTPStatus   *int               `json:"http_status"`
	ErrorCode    *string            `json:"error_code"`
	ErrorMessage *string            `json:"error_message"`
	RequestID    string             `json:"request_id"`
	SyncCounts   map[string]int     `json:"sync_counts"`
	Metadata     map[string]string  `json:"metadata"`
}

type SyncAttemptLog struct {
	AttemptID        string
	DevID            string
	Project          string
	Client           string
	DaemonID         string
	StartedAt        time.Time
	EndedAt          *time.Time
	Outcome          SyncAttemptOutcome
	HTTPStatus       *int
	ErrorCode        *string
	ErrorMessage     *string
	RequestID        string
	SyncCounts       map[string]int
	Metadata         map[string]string
	PortalUserID     *string
	PortalUserSource *string
}

type SyncAttemptStoreResult struct {
	AcceptedIDs  []string
	DuplicateIDs []string
}

type SyncAttemptRejected struct {
	AttemptID string `json:"attempt_id"`
	Error     string `json:"error"`
}

type SyncAttemptIngestResponse struct {
	AcceptedIDs  []string              `json:"accepted_ids"`
	DuplicateIDs []string              `json:"duplicate_ids"`
	Rejected     []SyncAttemptRejected `json:"rejected"`
}

type SyncAttemptSummaryQuery struct {
	Window    string
	Project   string
	DevID     string
	Client    string
	DaemonID  string
	Outcome   string
	ErrorCode string
}

type SyncAttemptSummaryFilter struct {
	Since     time.Time
	Project   string
	DevID     string
	Client    string
	DaemonID  string
	Outcome   string
	ErrorCode string
}

type SyncAttemptSummaryRecord struct {
	DevID     string
	Project   string
	Client    string
	DaemonID  string
	StartedAt time.Time
	Outcome   SyncAttemptOutcome
	ErrorCode *string
}

type SyncAttemptSummaryResponse struct {
	Windows []SyncAttemptWindowSummary `json:"windows"`
}

type SyncAttemptWindowSummary struct {
	Window        string                      `json:"window"`
	Total         int                         `json:"total"`
	Successes     int                         `json:"successes"`
	Failures      int                         `json:"failures"`
	FailureRate   float64                     `json:"failure_rate"`
	LastSuccessAt *time.Time                  `json:"last_success_at,omitempty"`
	LastFailureAt *time.Time                  `json:"last_failure_at,omitempty"`
	ByDeveloper   []SyncAttemptDimensionCount `json:"by_developer"`
	ByProject     []SyncAttemptDimensionCount `json:"by_project"`
	ByClient      []SyncAttemptDimensionCount `json:"by_client"`
	ByDaemon      []SyncAttemptDimensionCount `json:"by_daemon"`
	ByOutcome     []SyncAttemptDimensionCount `json:"by_outcome"`
	ByErrorCode   []SyncAttemptDimensionCount `json:"by_error_code"`
	TopErrors     []SyncAttemptDimensionCount `json:"top_errors"`
}

type SyncAttemptDimensionCount struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// ProjectSyncHealthRow is a raw DB row returned by SyncHealthByProject.
type ProjectSyncHealthRow struct {
	Project          string
	LastOutcome      SyncAttemptOutcome
	ContributorCount int
	LastActivityAt   time.Time
}

type ProjectSyncHealthProjection struct {
	Rows     []ProjectSyncHealthRow
	Degraded int
	Total    int
}

type UserSyncProjectionRow struct {
	PortalUserID         string
	IsActive             bool
	LatestEndedAt        *time.Time
	LatestOutcome        *SyncAttemptOutcome
	LatestSuccessEndedAt *time.Time
}

type UserSyncProjection struct {
	Rows []UserSyncProjectionRow
}

type UserSyncStatus string

const (
	UserSyncStatusLast24h  UserSyncStatus = "last_24h"
	UserSyncStatusInactive UserSyncStatus = "inactive"
	UserSyncStatusNever    UserSyncStatus = "never"
	UserSyncStatusUnknown  UserSyncStatus = "unknown"
)
