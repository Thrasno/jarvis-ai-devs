package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
)

var ErrProjectBlockInvalidRequest = model.ErrProjectBlockInvalidRequest

var ErrProjectBlockUnavailable = errors.New("project block governance is not configured")

var ErrProjectKeyLockBusy = repository.ErrProjectKeyLockBusy

type ProjectBlockedError struct {
	Command model.ProjectBlockCommand
}

func (e *ProjectBlockedError) Error() string {
	return "project is blocked"
}

type ProjectGovernanceService interface {
	BlockProject(ctx context.Context, actor model.AdminActor, project string, req model.ProjectBlockRequest) (model.ProjectBlockResponse, error)
	Status(ctx context.Context, project string) (model.ProjectBlockStatusResponse, error)
	Inbox(ctx context.Context, subject model.ProjectBlockAckSubject) ([]model.ProjectBlockCommand, error)
	Acknowledge(ctx context.Context, ack model.ProjectBlockAck) (model.ProjectBlockAck, error)
	ListQuarantines(ctx context.Context) ([]model.QuarantineSummary, error)
	QuarantineProgress(ctx context.Context, canonicalProjectKey string, generation int64, after string, limit int) (model.QuarantineProgressResponse, error)
}

func (s *projectGovernanceService) ListQuarantines(ctx context.Context) ([]model.QuarantineSummary, error) {
	if s.blockRepo == nil {
		return nil, ErrProjectBlockUnavailable
	}
	return s.blockRepo.ListQuarantines(ctx)
}

func (s *projectGovernanceService) Inbox(ctx context.Context, subject model.ProjectBlockAckSubject) ([]model.ProjectBlockCommand, error) {
	if s.blockRepo == nil || !subject.Valid() {
		return nil, ErrProjectBlockUnavailable
	}
	return s.blockRepo.ListInboxCommands(ctx, subject)
}

type projectGovernanceService struct {
	blockRepo repository.ProjectBlockRepository
	auditRepo repository.AuditRepository
	tx        repository.TxManager
}

func NewProjectGovernanceService(blockRepo repository.ProjectBlockRepository, auditRepo repository.AuditRepository, tx repository.TxManager) ProjectGovernanceService {
	return &projectGovernanceService{blockRepo: blockRepo, auditRepo: auditRepo, tx: tx}
}

func (s *projectGovernanceService) BlockProject(ctx context.Context, actor model.AdminActor, project string, req model.ProjectBlockRequest) (model.ProjectBlockResponse, error) {
	if s.blockRepo == nil {
		return model.ProjectBlockResponse{}, ErrProjectBlockUnavailable
	}
	// The admin confirms the exact project spelling. Nothing derives a key from
	// it: the block is matched against stored literals with plain equality, so a
	// derived key would quarantine a project nobody named.
	if err := req.Validate(project); err != nil {
		return model.ProjectBlockResponse{}, err
	}
	if s.tx == nil {
		return model.ProjectBlockResponse{}, ErrProjectBlockUnavailable
	}
	var block *model.ProjectBlock
	err := s.tx.WithinTx(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		if repos.ProjectBlocks == nil || repos.Audit == nil || repos.ProjectKeyLocks == nil {
			return ErrProjectBlockUnavailable
		}
		if err := repos.ProjectKeyLocks.LockCanonicalProjectKeys(ctx, []string{project}); err != nil {
			return err
		}
		if repos.ProjectIdentities == nil {
			return ErrProjectBlockUnavailable
		}
		if err := repos.ProjectIdentities.Register(ctx, project, "", time.Now().UTC()); err != nil {
			return err
		}
		var err error
		block, err = repos.ProjectBlocks.BlockProject(ctx, model.ProjectBlockCreate{
			Project:             project,
			CanonicalProjectKey: project,
			Action:              req.Action,
			Reason:              req.Reason,
			Confirmation:        req.Confirmation,
			ExportMarker:        req.ExportMarker,
			ActorUserID:         actor.UserID,
		})
		if err != nil {
			return err
		}
		return emitProjectBlockAudit(ctx, repos.Audit, actor, block, req)
	})
	if err != nil {
		return model.ProjectBlockResponse{}, err
	}
	return model.NewProjectBlockResponse(block), nil
}

func (s *projectGovernanceService) Status(ctx context.Context, project string) (model.ProjectBlockStatusResponse, error) {
	if s.blockRepo == nil {
		return model.ProjectBlockStatusResponse{}, ErrProjectBlockUnavailable
	}
	block, err := s.blockRepo.GetByCanonicalKey(ctx, project)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.ProjectBlockStatusResponse{Project: project, CanonicalProjectKey: project, Blocked: false}, nil
		}
		return model.ProjectBlockStatusResponse{}, err
	}
	cmd := block.Command().Redacted()
	resp := model.ProjectBlockStatusResponse{Project: block.Project, CanonicalProjectKey: block.CanonicalProjectKey, Blocked: true, Reason: block.Reason, Command: &cmd}
	ack, err := s.blockRepo.LatestAckForCommand(ctx, project, block.CommandID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return model.ProjectBlockStatusResponse{}, err
	}
	resp.Ack = model.NewProjectBlockAckStatus(ack)
	return resp, nil
}

func (s *projectGovernanceService) QuarantineProgress(ctx context.Context, canonicalProjectKey string, generation int64, after string, limit int) (model.QuarantineProgressResponse, error) {
	if s.tx == nil {
		return model.QuarantineProgressResponse{}, ErrProjectBlockUnavailable
	}
	var result model.QuarantineProgressResponse
	err := s.tx.ReadOnlyRepeatableRead(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		if repos.ProjectBlocks == nil {
			return ErrProjectBlockUnavailable
		}
		var err error
		result, err = repos.ProjectBlocks.QuarantineProgress(ctx, canonicalProjectKey, generation, after, limit)
		return err
	})
	return result, err
}

func (s *projectGovernanceService) Acknowledge(ctx context.Context, ack model.ProjectBlockAck) (model.ProjectBlockAck, error) {
	if s.blockRepo == nil {
		return model.ProjectBlockAck{}, ErrProjectBlockUnavailable
	}

	if err := ack.Validate(); err != nil {
		return model.ProjectBlockAck{}, err
	}
	if s.tx == nil {
		return model.ProjectBlockAck{}, ErrProjectBlockUnavailable
	}
	var recorded model.ProjectBlockAck
	err := s.tx.WithinTx(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		if repos.ProjectBlocks == nil || repos.ProjectKeyLocks == nil {
			return ErrProjectBlockUnavailable
		}
		if err := repos.ProjectKeyLocks.LockCanonicalProjectKeys(ctx, []string{ack.CanonicalProjectKey}); err != nil {
			return err
		}
		block, err := repos.ProjectBlocks.GetByCanonicalKey(ctx, ack.CanonicalProjectKey)
		if err != nil {
			if !errors.Is(err, repository.ErrNotFound) {
				return err
			}
			block = nil // UNBLOCK is already released from the current blocked head.
		}
		if (block != nil && block.CommandID != ack.CommandID) || !ack.AckSubject.Valid() {
			return ErrProjectBlockInvalidRequest
		}
		delivery, err := repos.ProjectBlocks.GetAckDelivery(ctx, ack.CanonicalProjectKey, ack.CommandID, ack.AckSubject)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return ErrProjectBlockInvalidRequest
			}
			return err
		}
		if delivery == nil || delivery.AckToken != ack.AckToken {
			return ErrProjectBlockInvalidRequest
		}
		ack.AppliedAt = time.Now().UTC()
		var recordErr error
		recorded, recordErr = repos.ProjectBlocks.RecordAck(ctx, ack)
		return recordErr
	})
	if err != nil {
		return model.ProjectBlockAck{}, err
	}
	return recorded, nil
}

func emitProjectBlockAudit(ctx context.Context, auditRepo repository.AuditRepository, actor model.AdminActor, block *model.ProjectBlock, req model.ProjectBlockRequest) error {
	if auditRepo == nil {
		return ErrProjectBlockUnavailable
	}
	if block == nil {
		return fmt.Errorf("project block audit requires persisted block")
	}
	project := block.CanonicalProjectKey
	actorID := actor.UserID
	if err := auditRepo.Insert(ctx, &model.AuditEntry{
		ActorUserID: &actorID,
		Project:     &project,
		Action:      model.AuditActionProjectBlock,
		Outcome:     model.AuditOutcomeSuccess,
		EntryCount:  1,
		Metadata: model.AuditMetadata{
			"project":    block.Project,
			"reason":     req.Reason,
			"action":     block.Action,
			"generation": block.Generation,
			"actor":      actor.UserID,
		},
	}); err != nil {
		return fmt.Errorf("record project block audit: %w", err)
	}
	return nil
}

func projectBlockedError(block *model.ProjectBlock) error {
	if block == nil {
		return nil
	}
	return &ProjectBlockedError{Command: block.Command()}
}

func projectBlockedErrorDelivery(ctx context.Context, blockRepo repository.ProjectBlockRepository, block *model.ProjectBlock, subject model.ProjectBlockAckSubject) error {
	if block == nil {
		return nil
	}
	if blockRepo == nil || !subject.Valid() {
		return projectBlockedError(block)
	}
	cmd, err := blockRepo.EnsureAckDelivery(ctx, block, subject)
	if err != nil {
		return err
	}
	return &ProjectBlockedError{Command: cmd}
}

func projectBlockedErrorForProjectDelivery(ctx context.Context, blockRepo repository.ProjectBlockRepository, project string, subject model.ProjectBlockAckSubject) error {
	if blockRepo == nil {
		return repository.ErrProjectBlocked
	}
	block, err := blockRepo.GetByCanonicalKey(ctx, project)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return repository.ErrProjectBlocked
		}
		return err
	}
	return projectBlockedErrorDelivery(ctx, blockRepo, block, subject)
}

func projectBlockedErrorForProject(ctx context.Context, blockRepo repository.ProjectBlockRepository, project string) error {
	if blockRepo == nil {
		return repository.ErrProjectBlocked
	}
	block, err := blockRepo.GetByCanonicalKey(ctx, project)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return repository.ErrProjectBlocked
		}
		return err
	}
	return projectBlockedError(block)
}
