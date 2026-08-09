package model

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const ProjectBlockProgressPending = "pending"

const (
	// ProjectBlockActionQuarantine remains readable for records written before
	// the distributed lifecycle. New writes use explicit BLOCK and UNBLOCK.
	ProjectBlockActionQuarantine  = "quarantine"
	ProjectBlockActionBlock       = "block"
	ProjectBlockActionUnblock     = "unblock"
	ProjectBlockActionPurgeIntent = "purge_intent"

	ProjectBlockAckApplied = "applied"
	ProjectBlockAckFailed  = "failed"
	ProjectBlockAckSkipped = "skipped"
)

var (
	ErrProjectBlockInvalidRequest = errors.New("project quarantine request requires a supported action, reason, and exact confirmation")
	ErrInvalidQuarantineCursor    = errors.New("invalid quarantine cursor")
)

type ProjectBlockRequest struct {
	Project      string `json:"project"`
	Action       string `json:"action" binding:"required"`
	Reason       string `json:"reason" binding:"required"`
	Confirmation string `json:"confirmation" binding:"required"`
	// ExportMarker is retained only to decode historical payloads and rows.
	ExportMarker string `json:"export_marker,omitempty"`
}

func (r ProjectBlockRequest) Validate(projectKey string) error {
	if strings.TrimSpace(r.Action) == "" || strings.TrimSpace(r.Reason) == "" {
		return ErrProjectBlockInvalidRequest
	}
	if r.Action != ProjectBlockActionBlock && r.Action != ProjectBlockActionUnblock {
		return ErrProjectBlockInvalidRequest
	}
	if r.Confirmation != projectKey {
		return ErrProjectBlockInvalidRequest
	}
	return nil
}

// Every ProjectKey in this file is the project literal an admin blocked, stored
// and matched verbatim. The `canonical_project_key` column and JSON name are
// historical: nothing in this module canonicalizes, folds or derives them. The
// column keeps its name because migrations 012 and 018 hang foreign keys and a
// trigger off it; the JSON name keeps it because the daemon and the dashboard
// already decode it.

type ProjectBlockCreate struct {
	Project      string
	ProjectKey   string
	Action       string
	Reason       string
	Confirmation string
	ExportMarker string
	ActorUserID  string
}

type ProjectBlock struct {
	ID           string    `json:"id"`
	CommandID    string    `json:"command_id"`
	AckToken     string    `json:"ack_token,omitempty"`
	Project      string    `json:"project"`
	ProjectKey   string    `json:"canonical_project_key"`
	Action       string    `json:"action"`
	Generation   int64     `json:"generation"`
	Reason       string    `json:"reason"`
	Confirmation string    `json:"confirmation"`
	ExportMarker string    `json:"export_marker"`
	ActorUserID  string    `json:"actor_user_id"`
	Blocked      bool      `json:"blocked"`
	BlockedAt    time.Time `json:"blocked_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ProjectBlockCommand struct {
	CommandID  string    `json:"command_id"`
	AckToken   string    `json:"ack_token,omitempty"`
	Project    string    `json:"project"`
	ProjectKey string    `json:"canonical_project_key"`
	Reason     string    `json:"reason"`
	Action     string    `json:"action"`
	Generation int64     `json:"generation"`
	BlockedAt  time.Time `json:"blocked_at"`
}

// Validate rejects malformed delivery commands before a daemon can mutate
// local state. Historical rows are readable, but they are never new commands.
func (c ProjectBlockCommand) Validate() error {
	if strings.TrimSpace(c.CommandID) == "" || strings.TrimSpace(c.AckToken) == "" || strings.TrimSpace(c.ProjectKey) == "" || c.Generation < 1 {
		return ErrProjectBlockInvalidRequest
	}
	if c.Action != ProjectBlockActionBlock && c.Action != ProjectBlockActionUnblock {
		return ErrProjectBlockInvalidRequest
	}
	return nil
}

type ProjectBlockAckSubject struct {
	AuthSubject string
	DaemonID    string
	Client      string
}

func (s ProjectBlockAckSubject) Valid() bool {
	return strings.TrimSpace(s.AuthSubject) != ""
}

func (s ProjectBlockAckSubject) Key() string {
	return strings.Join([]string{strings.TrimSpace(s.AuthSubject), strings.TrimSpace(s.DaemonID), strings.TrimSpace(s.Client)}, ":")
}

type ProjectBlockAckDelivery struct {
	CommandID  string
	ProjectKey string
	AckToken   string
	AckSubject ProjectBlockAckSubject
}

type ProjectBlockAck struct {
	CommandID  string                 `json:"command_id" binding:"required"`
	ProjectKey string                 `json:"canonical_project_key" binding:"required"`
	AckToken   string                 `json:"ack_token" binding:"required"`
	Status     string                 `json:"status" binding:"required"`
	Warning    string                 `json:"warning,omitempty"`
	AppliedAt  time.Time              `json:"applied_at"`
	AckSubject ProjectBlockAckSubject `json:"-"`
}

type ProjectBlockAckStatus struct {
	CommandID  string                 `json:"command_id"`
	ProjectKey string                 `json:"canonical_project_key"`
	Status     string                 `json:"status"`
	Warning    string                 `json:"warning,omitempty"`
	AppliedAt  time.Time              `json:"applied_at"`
	AckSubject ProjectBlockAckSubject `json:"-"`
}

func (a ProjectBlockAck) Validate() error {
	if strings.TrimSpace(a.CommandID) == "" || strings.TrimSpace(a.ProjectKey) == "" || strings.TrimSpace(a.AckToken) == "" {
		return ErrProjectBlockInvalidRequest
	}
	switch a.Status {
	case ProjectBlockAckApplied, ProjectBlockAckFailed, ProjectBlockAckSkipped:
		return nil
	default:
		return ErrProjectBlockInvalidRequest
	}
}

type ProjectBlockResponse struct {
	CommandID  string    `json:"command_id"`
	Project    string    `json:"project"`
	ProjectKey string    `json:"canonical_project_key"`
	Reason     string    `json:"reason"`
	BlockedAt  time.Time `json:"blocked_at"`
}

type ProjectBlockedErrorResponse struct {
	Error   string              `json:"error"`
	Command ProjectBlockCommand `json:"command"`
}

type ProjectBlockStatusResponse struct {
	Project    string                 `json:"project"`
	ProjectKey string                 `json:"canonical_project_key"`
	Blocked    bool                   `json:"blocked"`
	Reason     string                 `json:"reason,omitempty"`
	Command    *ProjectBlockCommand   `json:"command,omitempty"`
	Ack        *ProjectBlockAckStatus `json:"ack,omitempty"`
}

// QuarantineProgressResponse is the deliberately narrow admin-only projection.
type QuarantineProgressResponse struct {
	Project    string                   `json:"project"`
	ProjectKey string                   `json:"canonical_project_key"`
	Generation int64                    `json:"generation"`
	Action     string                   `json:"action"`
	Totals     QuarantineProgressTotals `json:"totals"`
	Progress   []QuarantineProgressRow  `json:"progress"`
	NextCursor string                   `json:"next_cursor,omitempty"`
}

type QuarantineProgressTotals struct {
	Active       int `json:"active"`
	Acknowledged int `json:"acknowledged"`
	Pending      int `json:"pending"`
}

type QuarantineProgressRow struct {
	Username       string     `json:"username"`
	State          string     `json:"state"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
}

type QuarantineSummary struct {
	Project        string    `json:"project"`
	ProjectKey     string    `json:"canonical_project_key"`
	Generation     int64     `json:"generation"`
	Action         string    `json:"action"`
	State          string    `json:"state"`
	TransitionedAt time.Time `json:"transitioned_at"`
}

type QuarantineCursor struct {
	ProjectKey string `json:"canonical_project_key"`
	Generation int64  `json:"generation"`
	Username   string `json:"username"`
	// CursorID is a one-way ordering key. It preserves stable pagination without
	// serializing an account ID into the admin response cursor.
	CursorID string `json:"cursor_id"`
}

func (c QuarantineCursor) Encode() string {
	encoded, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func DecodeQuarantineCursor(value, projectKey string, generation int64) (QuarantineCursor, error) {
	if value == "" {
		return QuarantineCursor{}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return QuarantineCursor{}, ErrInvalidQuarantineCursor
	}
	var cursor QuarantineCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.ProjectKey != projectKey || cursor.Generation != generation || cursor.CursorID == "" {
		return QuarantineCursor{}, ErrInvalidQuarantineCursor
	}
	return cursor, nil
}

func (b ProjectBlock) Command() ProjectBlockCommand {
	return ProjectBlockCommand{CommandID: b.CommandID, AckToken: b.AckToken, Project: b.Project, ProjectKey: b.ProjectKey, Reason: b.Reason, Action: b.Action, Generation: b.Generation, BlockedAt: b.BlockedAt}
}

func (c ProjectBlockCommand) Redacted() ProjectBlockCommand {
	c.AckToken = ""
	c.Reason = ""
	return c
}

func NewProjectBlockResponse(block *ProjectBlock) ProjectBlockResponse {
	if block == nil {
		return ProjectBlockResponse{}
	}
	return ProjectBlockResponse{CommandID: block.CommandID, Project: block.Project, ProjectKey: block.ProjectKey, Reason: block.Reason, BlockedAt: block.BlockedAt}
}

func NewProjectBlockAckStatus(ack *ProjectBlockAck) *ProjectBlockAckStatus {
	if ack == nil {
		return nil
	}
	return &ProjectBlockAckStatus{CommandID: ack.CommandID, ProjectKey: ack.ProjectKey, Status: ack.Status, Warning: ack.Warning, AppliedAt: ack.AppliedAt, AckSubject: ack.AckSubject}
}
