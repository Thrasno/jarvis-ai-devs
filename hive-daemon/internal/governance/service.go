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
	ErrMemoryIDRequired                 = errors.New("memory id is required")
	ErrMemoryNotFound                   = errors.New("memory not found")
	ErrDestructiveBackupRequired        = errors.New("fresh backup is required before destructive operation")
	ErrDestructiveConfirmationRequired  = errors.New("destructive operation confirmation is required")
	ErrDestructiveConfirmationMismatch  = errors.New("destructive operation confirmation mismatch")
	ErrDestructiveReasonRequired        = errors.New("delete reason is required")
	ErrDestructiveTargetRequired        = errors.New("destructive operation target is required")
	ErrDestructiveOperationUnsupported  = errors.New("destructive operation is unsupported")
	ErrDestructiveMutationStoreRequired = errors.New("destructive mutation store is not configured")
)

const (
	GuardOperationDelete  = "delete"
	GuardOperationRestore = "restore"
	GuardOperationArchive = "archive"
	GuardOperationMerge   = "merge"
	GuardTargetMemory     = "memory"
	GuardTargetProject    = "project"

	projectArchiveCloudHandoffNote = "Local project archive completed. No cloud project mutation was performed; review Hive API/dashboard handoff separately if shared project state must change."
	projectMergeCloudHandoffNote   = "Local project merge metadata recorded. No cloud project mutation was performed; review Hive API/dashboard handoff separately if shared project state must change."

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
	GetGovernanceMemoryByID(context.Context, int64) (db.GovernanceMemory, error)
	ListGovernanceSyncHealth(context.Context) ([]db.SyncHealth, error)
	ListHiveWarnings(db.HiveWarningFilter) ([]db.HiveWarning, error)
}

type backupStore interface {
	List(context.Context) ([]BackupManifest, error)
	Create(context.Context) (BackupManifest, error)
	PlanRestore(context.Context, RestoreRequest) (RestoreResult, error)
	ValidateArchive(context.Context, string) (BackupManifest, error)
}

type memoryMutationStore interface {
	DeleteMemory(id int64, actorID, reason string) error
	RestoreMemory(id int64, actorID string) error
}

type projectArchiveStore interface {
	ArchiveGovernanceProject(ctx context.Context, project, actorID, reason string, archivedAt time.Time) (bool, error)
}

type projectMergeStore interface {
	MergeGovernanceProject(ctx context.Context, source, target, actorID, reason string, mergedAt time.Time) (bool, error)
	ProjectMergeSyncEvidence(ctx context.Context, projects []string) (bool, error)
}

type projectDeleteStore interface {
	DeleteGovernanceProject(ctx context.Context, project, actorID, reason string) (int, error)
}

// MergeResult holds the outcome of a single source→target merge within a batch.
type MergeResult struct {
	Source        string `json:"source"`
	Target        string `json:"target"`
	AlreadyMerged bool   `json:"already_merged"`
	Mutated       bool   `json:"mutated"`
	ErrMsg        string `json:"error,omitempty"`
}

// ProjectMergeBatchRequest is the input for a multi-source merge batch.
type ProjectMergeBatchRequest struct {
	Sources      []string `json:"sources"`
	Target       string   `json:"target"`
	BackupID     string   `json:"backup_id"`
	Confirmation string   `json:"confirmation"`
	ActorID      string   `json:"actor_id,omitempty"`
	Reason       string   `json:"reason,omitempty"`
}

// ProjectMergeBatchResult is the output of a multi-source merge batch.
type ProjectMergeBatchResult struct {
	Operation        string        `json:"operation"`
	Target           string        `json:"target"`
	BackupID         string        `json:"backup_id"`
	Results          []MergeResult `json:"results"`
	HasSyncEvidence  bool          `json:"has_sync_evidence"`
	CloudHandoffNote string        `json:"cloud_handoff_note,omitempty"`
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

type ProjectArchiveRequest struct {
	Project      string `json:"project"`
	BackupID     string `json:"backup_id"`
	Confirmation string `json:"confirmation"`
	ActorID      string `json:"actor_id,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

type ProjectArchiveResult struct {
	Operation        string `json:"operation"`
	TargetType       string `json:"target_type"`
	Project          string `json:"project"`
	BackupID         string `json:"backup_id"`
	Mutated          bool   `json:"mutated"`
	CloudHandoffNote string `json:"cloud_handoff_note"`
}

type ProjectMergeRequest struct {
	SourceProject string `json:"source_project"`
	TargetProject string `json:"target_project"`
	BackupID      string `json:"backup_id"`
	Confirmation  string `json:"confirmation"`
	ActorID       string `json:"actor_id,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type ProjectMergeResult struct {
	Operation        string `json:"operation"`
	TargetType       string `json:"target_type"`
	SourceProject    string `json:"source_project"`
	TargetProject    string `json:"target_project"`
	BackupID         string `json:"backup_id"`
	Mutated          bool   `json:"mutated"`
	CloudHandoffNote string `json:"cloud_handoff_note"`
}

// ProjectDeleteRequest is the input for ExecuteProjectDelete.
type ProjectDeleteRequest struct {
	Project      string `json:"project"`
	BackupID     string `json:"backup_id"`
	Confirmation string `json:"confirmation"`
	ActorID      string `json:"actor_id,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

// DeleteProjectResult is the output of ExecuteProjectDelete.
type DeleteProjectResult struct {
	Operation        string `json:"operation"`
	TargetType       string `json:"target_type"`
	Project          string `json:"project"`
	BackupID         string `json:"backup_id"`
	RowsDeleted      int    `json:"rows_deleted"`
	Mutated          bool   `json:"mutated"`
	CloudHandoffNote string `json:"cloud_handoff_note"`
}

type Service struct {
	store   readStore
	backup  backupStore
	now     func() time.Time
	imports *engramImportJobManager
}

func NewService(store readStore) *Service {
	return &Service{store: store, now: time.Now, imports: newEngramImportJobManager()}
}

func NewServiceWithBackup(store readStore, backup backupStore) *Service {
	return &Service{store: store, backup: backup, now: time.Now, imports: newEngramImportJobManager()}
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

func (s *Service) Warnings(ctx context.Context, filter WarningFilter) ([]Warning, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, fmt.Errorf("warning store is not configured")
	}
	warnings, err := s.store.ListHiveWarnings(db.HiveWarningFilter{ResolutionState: strings.TrimSpace(filter.ResolutionState)})
	if err != nil {
		return nil, err
	}
	result := make([]Warning, 0, len(warnings))
	for _, warning := range warnings {
		result = append(result, warningFromDB(warning))
	}
	return result, nil
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
	reason := strings.TrimSpace(req.Reason)
	if req.TargetID <= 0 {
		return GuardResult{}, ErrDestructiveTargetRequired
	}
	if operation != GuardOperationDelete && operation != GuardOperationRestore || targetType != GuardTargetMemory {
		return GuardResult{}, ErrDestructiveOperationUnsupported
	}
	if operation == GuardOperationDelete && reason == "" {
		return GuardResult{}, ErrDestructiveReasonRequired
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
		if err := mutator.DeleteMemory(req.TargetID, req.ActorID, reason); err != nil {
			return GuardResult{}, err
		}
	case GuardOperationRestore:
		if err := mutator.RestoreMemory(req.TargetID, req.ActorID); err != nil {
			return GuardResult{}, err
		}
	}
	return GuardResult{Operation: operation, TargetType: targetType, TargetID: req.TargetID, BackupID: backupID, Mutated: true}, nil
}

func (s *Service) ExecuteProjectArchive(ctx context.Context, req ProjectArchiveRequest) (ProjectArchiveResult, error) {
	project := strings.TrimSpace(req.Project)
	if project == "" {
		return ProjectArchiveResult{}, ErrProjectRequired
	}
	backupID, err := s.requireFreshBackup(ctx, req.BackupID)
	if err != nil {
		return ProjectArchiveResult{}, err
	}
	if req.Confirmation == "" {
		return ProjectArchiveResult{}, ErrDestructiveConfirmationRequired
	}
	if req.Confirmation != ProjectArchiveConfirmation(project) {
		return ProjectArchiveResult{}, ErrDestructiveConfirmationMismatch
	}
	archiver, ok := s.store.(projectArchiveStore)
	if !ok {
		return ProjectArchiveResult{}, ErrDestructiveMutationStoreRequired
	}
	mutated, err := archiver.ArchiveGovernanceProject(ctx, project, req.ActorID, req.Reason, s.currentTime().UTC())
	if err != nil {
		return ProjectArchiveResult{}, mapProjectError(err)
	}
	return ProjectArchiveResult{
		Operation:        GuardOperationArchive,
		TargetType:       GuardTargetProject,
		Project:          project,
		BackupID:         backupID,
		Mutated:          mutated,
		CloudHandoffNote: projectArchiveCloudHandoffNote,
	}, nil
}

const projectDeleteCloudHandoffNote = "Project purged locally. Cloud data not removed — no tombstone sync protocol exists yet."

// ExecuteProjectDelete irreversibly purges all local data for an archived project.
// Guards: fresh backup, exact confirmation phrase ("PURGE project <name>").
// Propagates db.ErrGovernanceProjectNotArchived as-is for HTTP 409 mapping.
func (s *Service) ExecuteProjectDelete(ctx context.Context, req ProjectDeleteRequest) (DeleteProjectResult, error) {
	project := strings.TrimSpace(req.Project)
	if project == "" {
		return DeleteProjectResult{}, ErrProjectRequired
	}
	backupID, err := s.requireFreshBackup(ctx, req.BackupID)
	if err != nil {
		return DeleteProjectResult{}, err
	}
	if strings.TrimSpace(req.Confirmation) == "" {
		return DeleteProjectResult{}, ErrDestructiveConfirmationRequired
	}
	if req.Confirmation != ProjectDeleteConfirmation(project) {
		return DeleteProjectResult{}, ErrDestructiveConfirmationMismatch
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return DeleteProjectResult{}, ErrDestructiveReasonRequired
	}
	deleter, ok := s.store.(projectDeleteStore)
	if !ok {
		return DeleteProjectResult{}, ErrDestructiveMutationStoreRequired
	}
	rowsDeleted, err := deleter.DeleteGovernanceProject(ctx, project, req.ActorID, reason)
	if err != nil {
		// Propagate ErrGovernanceProjectNotArchived as-is (HTTP layer maps to 409).
		return DeleteProjectResult{}, err
	}
	return DeleteProjectResult{
		Operation:        "purge",
		TargetType:       GuardTargetProject,
		Project:          project,
		BackupID:         backupID,
		RowsDeleted:      rowsDeleted,
		Mutated:          rowsDeleted > 0,
		CloudHandoffNote: projectDeleteCloudHandoffNote,
	}, nil
}

func (s *Service) ExecuteProjectMerge(ctx context.Context, req ProjectMergeRequest) (ProjectMergeResult, error) {
	source := strings.TrimSpace(req.SourceProject)
	target := strings.TrimSpace(req.TargetProject)
	if source == "" || target == "" {
		return ProjectMergeResult{}, ErrProjectRequired
	}
	if source == target {
		return ProjectMergeResult{}, db.ErrGovernanceProjectMergeInvalid
	}
	if req.SourceProject != source || req.TargetProject != target {
		return ProjectMergeResult{}, ErrProjectRequired
	}
	backupID, err := s.requireFreshBackup(ctx, req.BackupID)
	if err != nil {
		return ProjectMergeResult{}, err
	}
	if req.Confirmation == "" {
		return ProjectMergeResult{}, ErrDestructiveConfirmationRequired
	}
	if req.Confirmation != ProjectMergeConfirmation(source, target) {
		return ProjectMergeResult{}, ErrDestructiveConfirmationMismatch
	}
	merger, ok := s.store.(projectMergeStore)
	if !ok {
		return ProjectMergeResult{}, ErrDestructiveMutationStoreRequired
	}
	mutated, err := merger.MergeGovernanceProject(ctx, source, target, req.ActorID, req.Reason, s.currentTime().UTC())
	if err != nil {
		return ProjectMergeResult{}, mapProjectError(err)
	}
	return ProjectMergeResult{
		Operation:        GuardOperationMerge,
		TargetType:       GuardTargetProject,
		SourceProject:    source,
		TargetProject:    target,
		BackupID:         backupID,
		Mutated:          mutated,
		CloudHandoffNote: projectMergeCloudHandoffNote,
	}, nil
}

func GuardConfirmation(operation, targetType string, targetID int64) string {
	return fmt.Sprintf("%s %s %d", strings.ToUpper(normalizeGuardPart(operation)), normalizeGuardPart(targetType), targetID)
}

func ProjectArchiveConfirmation(project string) string {
	return fmt.Sprintf("%s %s %s", strings.ToUpper(GuardOperationArchive), GuardTargetProject, strings.TrimSpace(project))
}

func ProjectMergeConfirmation(source, target string) string {
	return fmt.Sprintf("%s %s %s INTO %s", strings.ToUpper(GuardOperationMerge), GuardTargetProject, strings.TrimSpace(source), strings.TrimSpace(target))
}

// ProjectDeleteConfirmation returns the exact phrase the caller must type to
// confirm a project purge. Mirrors the ARCHIVE phrase style.
func ProjectDeleteConfirmation(p string) string {
	return "PURGE project " + strings.TrimSpace(p)
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
		if _, err := s.backup.ValidateArchive(ctx, backupID); err != nil {
			return "", err
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

func (s *Service) MemoryByID(ctx context.Context, id int64) (Memory, error) {
	if id <= 0 {
		return Memory{}, ErrMemoryIDRequired
	}
	memory, err := s.store.GetGovernanceMemoryByID(ctx, id)
	return memory, mapMemoryError(err)
}

func mapMemoryError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, db.ErrGovernanceMemoryNotFound) {
		return fmt.Errorf("%w: %v", ErrMemoryNotFound, err)
	}
	return err
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

// ProjectMergeBatchConfirmation returns the exact confirmation phrase required
// for a multi-source batch merge. The caller must type this phrase verbatim.
func ProjectMergeBatchConfirmation(target string) string {
	return fmt.Sprintf("MERGE projects INTO %s", strings.TrimSpace(target))
}

// ExecuteProjectMergeBatch takes a single backup, checks for cloud sync evidence,
// then merges each source into target serially. A failure on one source is
// recorded in Results and does not prevent the remaining sources from being merged.
// Top-level errors (backup failure, validation failure) abort the entire batch.
func (s *Service) ExecuteProjectMergeBatch(ctx context.Context, req ProjectMergeBatchRequest) (ProjectMergeBatchResult, error) {
	target := strings.TrimSpace(req.Target)
	if target == "" {
		return ProjectMergeBatchResult{}, ErrProjectRequired
	}
	if len(req.Sources) == 0 {
		return ProjectMergeBatchResult{}, ErrProjectRequired
	}
	// Deduplicate and trim sources.
	seen := make(map[string]struct{}, len(req.Sources))
	sources := make([]string, 0, len(req.Sources))
	for _, s := range req.Sources {
		s = strings.TrimSpace(s)
		if s == "" || s == target {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		sources = append(sources, s)
	}
	if len(sources) == 0 {
		return ProjectMergeBatchResult{}, ErrProjectRequired
	}

	// Validate confirmation phrase.
	if req.Confirmation == "" {
		return ProjectMergeBatchResult{}, ErrDestructiveConfirmationRequired
	}
	if req.Confirmation != ProjectMergeBatchConfirmation(target) {
		return ProjectMergeBatchResult{}, ErrDestructiveConfirmationMismatch
	}

	// Single backup gates the entire batch.
	backupID, err := s.requireFreshBackup(ctx, req.BackupID)
	if err != nil {
		return ProjectMergeBatchResult{}, err
	}

	merger, ok := s.store.(projectMergeStore)
	if !ok {
		return ProjectMergeBatchResult{}, ErrDestructiveMutationStoreRequired
	}

	// Cloud sync evidence check (once, before loop).
	allProjects := make([]string, 0, len(sources)+1)
	allProjects = append(allProjects, sources...)
	allProjects = append(allProjects, target)
	hasSyncEvidence, err := merger.ProjectMergeSyncEvidence(ctx, allProjects)
	if err != nil {
		return ProjectMergeBatchResult{}, fmt.Errorf("sync evidence check: %w", err)
	}

	actorID := strings.TrimSpace(req.ActorID)
	reason := strings.TrimSpace(req.Reason)
	now := s.currentTime().UTC()

	results := make([]MergeResult, 0, len(sources))
	for _, source := range sources {
		mutated, mergeErr := merger.MergeGovernanceProject(ctx, source, target, actorID, reason, now)
		r := MergeResult{
			Source:  source,
			Target:  target,
			Mutated: mutated,
		}
		if mergeErr != nil {
			r.ErrMsg = mergeErr.Error()
		} else if !mutated {
			r.AlreadyMerged = true
		}
		results = append(results, r)
	}

	var cloudNote string
	if hasSyncEvidence {
		cloudNote = projectMergeCloudHandoffNote
	}
	return ProjectMergeBatchResult{
		Operation:        GuardOperationMerge,
		Target:           target,
		BackupID:         backupID,
		Results:          results,
		HasSyncEvidence:  hasSyncEvidence,
		CloudHandoffNote: cloudNote,
	}, nil
}
