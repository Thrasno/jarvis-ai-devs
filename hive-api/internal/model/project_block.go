package model

import (
	"errors"
	"strings"
	"time"
)

const (
	ProjectBlockActionQuarantine  = "quarantine"
	ProjectBlockActionPurgeIntent = "purge_intent"

	ProjectBlockAckApplied = "applied"
	ProjectBlockAckFailed  = "failed"
	ProjectBlockAckSkipped = "skipped"
)

var ErrProjectBlockInvalidRequest = errors.New("project block request requires action, reason, exact confirmation, and export marker")

type ProjectBlockRequest struct {
	Project      string `json:"project"`
	Action       string `json:"action" binding:"required"`
	Reason       string `json:"reason" binding:"required"`
	Confirmation string `json:"confirmation" binding:"required"`
	ExportMarker string `json:"export_marker" binding:"required"`
}

func (r ProjectBlockRequest) Validate(canonicalProjectKey string) error {
	if strings.TrimSpace(r.Action) == "" || strings.TrimSpace(r.Reason) == "" || strings.TrimSpace(r.ExportMarker) == "" {
		return ErrProjectBlockInvalidRequest
	}
	if r.Action != ProjectBlockActionQuarantine && r.Action != ProjectBlockActionPurgeIntent {
		return ErrProjectBlockInvalidRequest
	}
	if r.Confirmation != canonicalProjectKey {
		return ErrProjectBlockInvalidRequest
	}
	return nil
}

type ProjectBlockCreate struct {
	Project             string
	CanonicalProjectKey string
	Action              string
	Reason              string
	Confirmation        string
	ExportMarker        string
	ActorUserID         string
}

type ProjectBlock struct {
	ID                  string    `json:"id"`
	CommandID           string    `json:"command_id"`
	AckToken            string    `json:"ack_token,omitempty"`
	Project             string    `json:"project"`
	CanonicalProjectKey string    `json:"canonical_project_key"`
	Action              string    `json:"action"`
	Reason              string    `json:"reason"`
	Confirmation        string    `json:"confirmation"`
	ExportMarker        string    `json:"export_marker"`
	ActorUserID         string    `json:"actor_user_id"`
	Blocked             bool      `json:"blocked"`
	BlockedAt           time.Time `json:"blocked_at"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type ProjectBlockCommand struct {
	CommandID           string    `json:"command_id"`
	AckToken            string    `json:"ack_token,omitempty"`
	Project             string    `json:"project"`
	CanonicalProjectKey string    `json:"canonical_project_key"`
	Reason              string    `json:"reason"`
	BlockedAt           time.Time `json:"blocked_at"`
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
	CommandID           string
	CanonicalProjectKey string
	AckToken            string
	AckSubject          ProjectBlockAckSubject
}

type ProjectBlockAck struct {
	CommandID           string                 `json:"command_id" binding:"required"`
	CanonicalProjectKey string                 `json:"canonical_project_key" binding:"required"`
	AckToken            string                 `json:"ack_token" binding:"required"`
	Status              string                 `json:"status" binding:"required"`
	Warning             string                 `json:"warning,omitempty"`
	AppliedAt           time.Time              `json:"applied_at"`
	AckSubject          ProjectBlockAckSubject `json:"-"`
}

type ProjectBlockAckStatus struct {
	CommandID           string                 `json:"command_id"`
	CanonicalProjectKey string                 `json:"canonical_project_key"`
	Status              string                 `json:"status"`
	Warning             string                 `json:"warning,omitempty"`
	AppliedAt           time.Time              `json:"applied_at"`
	AckSubject          ProjectBlockAckSubject `json:"-"`
}

func (a ProjectBlockAck) Validate() error {
	if strings.TrimSpace(a.CommandID) == "" || strings.TrimSpace(a.CanonicalProjectKey) == "" || strings.TrimSpace(a.AckToken) == "" {
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
	CommandID           string    `json:"command_id"`
	Project             string    `json:"project"`
	CanonicalProjectKey string    `json:"canonical_project_key"`
	Reason              string    `json:"reason"`
	BlockedAt           time.Time `json:"blocked_at"`
}

type ProjectBlockedErrorResponse struct {
	Error   string              `json:"error"`
	Command ProjectBlockCommand `json:"command"`
}

type ProjectBlockStatusResponse struct {
	Project             string                 `json:"project"`
	CanonicalProjectKey string                 `json:"canonical_project_key"`
	Blocked             bool                   `json:"blocked"`
	Reason              string                 `json:"reason,omitempty"`
	Command             *ProjectBlockCommand   `json:"command,omitempty"`
	Ack                 *ProjectBlockAckStatus `json:"ack,omitempty"`
}

func (b ProjectBlock) Command() ProjectBlockCommand {
	return ProjectBlockCommand{CommandID: b.CommandID, AckToken: b.AckToken, Project: b.Project, CanonicalProjectKey: b.CanonicalProjectKey, Reason: b.Reason, BlockedAt: b.BlockedAt}
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
	return ProjectBlockResponse{CommandID: block.CommandID, Project: block.Project, CanonicalProjectKey: block.CanonicalProjectKey, Reason: block.Reason, BlockedAt: block.BlockedAt}
}

func NewProjectBlockAckStatus(ack *ProjectBlockAck) *ProjectBlockAckStatus {
	if ack == nil {
		return nil
	}
	return &ProjectBlockAckStatus{CommandID: ack.CommandID, CanonicalProjectKey: ack.CanonicalProjectKey, Status: ack.Status, Warning: ack.Warning, AppliedAt: ack.AppliedAt, AckSubject: ack.AckSubject}
}
