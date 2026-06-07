package governance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
)

type Project = db.GovernanceProject
type Memory = db.GovernanceMemory
type Health = db.SyncHealth

var (
	ErrProjectRequired                  = errors.New("project is required")
	ErrProjectNotFound                  = errors.New("project not found")
	ErrDestructiveBackupRequired        = errors.New("fresh backup is required before destructive operation")
	ErrDestructiveConfirmationRequired  = errors.New("destructive operation confirmation is required")
	ErrDestructiveConfirmationMismatch  = errors.New("destructive operation confirmation mismatch")
	ErrDestructiveTargetRequired        = errors.New("destructive operation target is required")
	ErrDestructiveOperationUnsupported  = errors.New("destructive operation is unsupported")
	ErrDestructiveMutationStoreRequired = errors.New("destructive mutation store is not configured")
)

const (
	GuardOperationDelete  = "delete"
	GuardOperationRestore = "restore"
	GuardTargetMemory     = "memory"

	destructiveBackupFreshness = 10 * time.Minute
)

type MemoryFilter struct {
	Project        string
	IncludeDeleted bool
	Limit          int
}

type readStore interface {
	ListGovernanceProjects(context.Context) ([]db.GovernanceProject, error)
	GetGovernanceProject(context.Context, string) (db.GovernanceProject, error)
	ListGovernanceMemories(context.Context, db.GovernanceMemoryFilter) ([]db.GovernanceMemory, error)
	ListGovernanceSyncHealth(context.Context) ([]db.SyncHealth, error)
}

type backupStore interface {
	List(context.Context) ([]BackupManifest, error)
	Create(context.Context) (BackupManifest, error)
	PlanRestore(context.Context, RestoreRequest) (RestoreResult, error)
}

type memoryMutationStore interface {
	DeleteMemory(id int64, actorID, reason string) error
	RestoreMemory(id int64, actorID string) error
}

type GuardRequest struct {
	Operation    string `json:"operation"`
	TargetType   string `json:"target_type"`
	TargetID     int64  `json:"target_id"`
	BackupID     string `json:"backup_id"`
	Confirmation string `json:"confirmation"`
	ActorID      string `json:"actor_id,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

type GuardResult struct {
	Operation  string `json:"operation"`
	TargetType string `json:"target_type"`
	TargetID   int64  `json:"target_id"`
	BackupID   string `json:"backup_id"`
	Mutated    bool   `json:"mutated"`
}

type Service struct {
	store  readStore
	backup backupStore
	now    func() time.Time
}

func NewService(store readStore) *Service {
	return &Service{store: store, now: time.Now}
}

func NewServiceWithBackup(store readStore, backup backupStore) *Service {
	return &Service{store: store, backup: backup, now: time.Now}
}

func (s *Service) Projects(ctx context.Context) ([]Project, error) {
	return s.store.ListGovernanceProjects(ctx)
}

func (s *Service) Project(ctx context.Context, name string) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, ErrProjectRequired
	}
	project, err := s.store.GetGovernanceProject(ctx, name)
	return project, mapProjectError(err)
}

func (s *Service) Memories(ctx context.Context, filter MemoryFilter) ([]Memory, error) {
	project := strings.TrimSpace(filter.Project)
	if project == "" {
		return nil, ErrProjectRequired
	}
	if _, err := s.store.GetGovernanceProject(ctx, project); err != nil {
		return nil, mapProjectError(err)
	}
	return s.store.ListGovernanceMemories(ctx, db.GovernanceMemoryFilter{
		Project:        project,
		IncludeDeleted: filter.IncludeDeleted,
		Limit:          filter.Limit,
	})
}

func (s *Service) Health(ctx context.Context) ([]Health, error) {
	return s.store.ListGovernanceSyncHealth(ctx)
}

func (s *Service) Backups(ctx context.Context) ([]BackupManifest, error) {
	if s.backup == nil {
		return nil, ErrBackupStoreRequired
	}
	return s.backup.List(ctx)
}

func (s *Service) CreateBackup(ctx context.Context) (BackupManifest, error) {
	if s.backup == nil {
		return BackupManifest{}, ErrBackupStoreRequired
	}
	return s.backup.Create(ctx)
}

func (s *Service) RestoreBackup(ctx context.Context, req RestoreRequest) (RestoreResult, error) {
	if s.backup == nil {
		return RestoreResult{}, ErrBackupStoreRequired
	}
	return s.backup.PlanRestore(ctx, req)
}

func (s *Service) ExecuteGuard(ctx context.Context, req GuardRequest) (GuardResult, error) {
	operation := normalizeGuardPart(req.Operation)
	targetType := normalizeGuardPart(req.TargetType)
	if req.TargetID <= 0 {
		return GuardResult{}, ErrDestructiveTargetRequired
	}
	if operation != GuardOperationDelete && operation != GuardOperationRestore || targetType != GuardTargetMemory {
		return GuardResult{}, ErrDestructiveOperationUnsupported
	}
	backupID, err := s.requireFreshBackup(ctx, req.BackupID)
	if err != nil {
		return GuardResult{}, err
	}
	if req.Confirmation == "" {
		return GuardResult{}, ErrDestructiveConfirmationRequired
	}
	if req.Confirmation != GuardConfirmation(operation, targetType, req.TargetID) {
		return GuardResult{}, ErrDestructiveConfirmationMismatch
	}
	mutator, ok := s.store.(memoryMutationStore)
	if !ok {
		return GuardResult{}, ErrDestructiveMutationStoreRequired
	}
	switch operation {
	case GuardOperationDelete:
		if err := mutator.DeleteMemory(req.TargetID, req.ActorID, req.Reason); err != nil {
			return GuardResult{}, err
		}
	case GuardOperationRestore:
		if err := mutator.RestoreMemory(req.TargetID, req.ActorID); err != nil {
			return GuardResult{}, err
		}
	}
	return GuardResult{Operation: operation, TargetType: targetType, TargetID: req.TargetID, BackupID: backupID, Mutated: true}, nil
}

func GuardConfirmation(operation, targetType string, targetID int64) string {
	return fmt.Sprintf("%s %s %d", strings.ToUpper(normalizeGuardPart(operation)), normalizeGuardPart(targetType), targetID)
}

func (s *Service) requireFreshBackup(ctx context.Context, backupID string) (string, error) {
	if s.backup == nil {
		return "", ErrBackupStoreRequired
	}
	backupID = strings.TrimSpace(backupID)
	if backupID == "" {
		return "", ErrDestructiveBackupRequired
	}
	backups, err := s.backup.List(ctx)
	if err != nil {
		return "", err
	}
	now := s.currentTime().UTC()
	for _, backup := range backups {
		if strings.TrimSpace(backup.ID) != backupID {
			continue
		}
		createdAt := backup.CreatedAt.UTC()
		if createdAt.IsZero() || createdAt.After(now) || now.Sub(createdAt) > destructiveBackupFreshness {
			return "", ErrDestructiveBackupRequired
		}
		return backup.ID, nil
	}
	return "", ErrDestructiveBackupRequired
}

func (s *Service) currentTime() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

func normalizeGuardPart(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func mapProjectError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, db.ErrGovernanceProjectRequired) {
		return ErrProjectRequired
	}
	if errors.Is(err, db.ErrGovernanceProjectNotFound) {
		return fmt.Errorf("%w: %v", ErrProjectNotFound, err)
	}
	return err
}
