package governance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"unicode"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
)

var (
	ErrSDDChangeRequired = errors.New("change is required")
	ErrSDDChangeInvalid  = errors.New("change is invalid")
	ErrSDDLimitInvalid   = errors.New("limit must be between 1 and 100")
	ErrSDDCursorInvalid  = errors.New("cursor is invalid")
)

var sddArtifactVocabulary = []string{
	"explore",
	"proposal",
	"spec",
	"design",
	"tasks",
	"apply-progress",
	"verify-report",
	"archive-report",
}

type sddStore interface {
	FetchSDDArtifacts(project, change string, artifacts []string) ([]db.SDDArtifact, error)
	ListSDDChanges(project string, artifacts []string, after string, limit int) ([]string, error)
	GetApplyProgress(project, change string) (db.ApplyProgressState, error)
	GetApplyProgressSuccessorOccupancy(project, change string) (db.ApplyProgressSuccessorOccupancy, error)
	GetApplyProgressEvidence(project, change, batchID, expectedHeadDigest string) (applyprogress.Batch, error)
	GetApplyProgressReceipt(project, change, requestID string) (db.ApplyProgressReceipt, error)
	AdvanceApplyProgress(db.ApplyProgressAdvance) (db.ApplyProgressAdvanceResult, error)
	PublishApplyProgressSuccessor(project, predecessorChange string) (db.ApplyProgressAdvanceResult, error)
	GetSDDStoreBinding(context.Context, string, string) (db.SDDStoreBinding, bool, error)
	AdoptSDDStoreBinding(context.Context, string, string, db.SDDStoreBindingRequest) (db.SDDStoreBinding, bool, error)
	ResolveProjectIngress(context.Context, string) (string, error)
}

type ApplyProgressAdvanceRequest = db.ApplyProgressAdvance
type ApplyProgressState = db.ApplyProgressState
type ApplyProgressSuccessorOccupancy = db.ApplyProgressSuccessorOccupancy
type ApplyProgressReceipt = db.ApplyProgressReceipt
type ApplyProgressAdvanceResult = db.ApplyProgressAdvanceResult
type SDDStoreBinding = db.SDDStoreBinding
type SDDStoreBindingRequest = db.SDDStoreBindingRequest
type SDDStoreBindingConflictError = db.SDDStoreBindingConflictError

// ProjectIngressError attaches the effective project key resolved at ingress
// (spelling fold plus alias) to a failure that happened after resolution, so
// transports can report which project identity was consulted.
type ProjectIngressError struct {
	ProjectKey string
	Err        error
}

func (e *ProjectIngressError) Error() string { return e.Err.Error() }

func (e *ProjectIngressError) Unwrap() error { return e.Err }

// ProjectKeyFromError returns the effective project key carried by err, if the
// failure happened after ingress resolution.
func ProjectKeyFromError(err error) (string, bool) {
	var ingress *ProjectIngressError
	if errors.As(err, &ingress) {
		return ingress.ProjectKey, true
	}
	return "", false
}

// withProjectKey wraps a post-resolution failure with the effective key.
func withProjectKey(key string, err error) error {
	if err == nil || key == "" {
		return err
	}
	return &ProjectIngressError{ProjectKey: key, Err: err}
}

// resolveApplyProgressProject resolves the effective project key once at the
// governance boundary (spelling fold plus alias) and applies the
// registered-project gate to that key, not to the spelling the caller sent: a
// retired key has no rows of its own once promoted. The database re-resolves
// inside its own transactions, so this key never authorizes a write by itself.
func (s *Service) resolveApplyProgressProject(ctx context.Context, project string) (string, error) {
	if s.sdd == nil {
		if _, err := s.store.GetGovernanceProject(ctx, project); err != nil {
			return "", mapProjectError(err)
		}
		return "", errors.New("SDD store is not configured")
	}
	key, err := s.sdd.ResolveProjectIngress(ctx, project)
	if err != nil {
		return "", err
	}
	if _, err := s.store.GetGovernanceProject(ctx, key); err != nil {
		return "", withProjectKey(key, mapProjectError(err))
	}
	return key, nil
}

// SDDArtifact is the daemon response projection for one known artifact.
type SDDArtifact struct {
	Artifact  string `json:"artifact"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type SDDChangePageRequest struct {
	Project string
	Limit   int
	Cursor  string
}

type SDDChangePage struct {
	Changes    []string `json:"changes"`
	NextCursor string   `json:"next_cursor,omitempty"`
}

type sddCursor struct {
	After string `json:"after"`
}

// GetSDDStoreBinding reads an immutable binding under the effective project key
// (spelling fold plus alias) without requiring an existing governance project or
// Hive artifact row. When found is false, the returned binding carries only the
// effective Project and Change coordinates. The database remains authoritative
// for project resolution.
func (s *Service) GetSDDStoreBinding(ctx context.Context, project, change string) (SDDStoreBinding, bool, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return SDDStoreBinding{}, false, ErrProjectRequired
	}
	change, err := validateSDDChange(change)
	if err != nil {
		return SDDStoreBinding{}, false, err
	}
	if s.sdd == nil {
		return SDDStoreBinding{}, false, errors.New("SDD store is not configured")
	}
	key, err := s.sdd.ResolveProjectIngress(ctx, project)
	if err != nil {
		return SDDStoreBinding{}, false, err
	}
	binding, found, err := s.sdd.GetSDDStoreBinding(ctx, key, change)
	if err != nil {
		return SDDStoreBinding{}, false, withProjectKey(key, err)
	}
	if !found {
		return SDDStoreBinding{Project: key, Change: change}, false, nil
	}
	return binding, true, nil
}

// AdoptSDDStoreBinding establishes the first immutable binding under the
// effective project key without requiring a prior governance project or Hive
// artifact row. A retired key binds its alias target, never itself.
func (s *Service) AdoptSDDStoreBinding(ctx context.Context, project, change string, request SDDStoreBindingRequest) (SDDStoreBinding, bool, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return SDDStoreBinding{}, false, ErrProjectRequired
	}
	change, err := validateSDDChange(change)
	if err != nil {
		return SDDStoreBinding{}, false, err
	}
	if s.sdd == nil {
		return SDDStoreBinding{}, false, errors.New("SDD store is not configured")
	}
	key, err := s.sdd.ResolveProjectIngress(ctx, project)
	if err != nil {
		return SDDStoreBinding{}, false, err
	}
	binding, created, err := s.sdd.AdoptSDDStoreBinding(ctx, key, change, request)
	return binding, created, withProjectKey(key, err)
}

func (s *Service) FetchSDDArtifacts(ctx context.Context, project, change string) ([]SDDArtifact, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return nil, ErrProjectRequired
	}
	change, err := validateSDDChange(change)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.GetGovernanceProject(ctx, project); err != nil {
		return nil, mapProjectError(err)
	}
	if s.sdd == nil {
		return nil, errors.New("SDD store is not configured")
	}
	rows, err := s.sdd.FetchSDDArtifacts(project, change, sddArtifactVocabulary)
	if err != nil {
		return nil, err
	}
	byTopic := make(map[string]db.SDDArtifact, len(rows))
	for _, row := range rows {
		byTopic[row.Topic] = row
	}
	result := make([]SDDArtifact, 0, len(rows))
	for _, artifact := range sddArtifactVocabulary {
		row, ok := byTopic["sdd/"+change+"/"+artifact]
		if !ok {
			continue
		}
		result = append(result, SDDArtifact{
			Artifact:  artifact,
			Content:   row.Content,
			CreatedAt: row.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return result, nil
}

// GetApplyProgress returns the snapshot selected by the guarded v2 head.
func (s *Service) GetApplyProgress(ctx context.Context, project, change string) (ApplyProgressState, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return ApplyProgressState{}, ErrProjectRequired
	}
	change, err := validateSDDChange(change)
	if err != nil {
		return ApplyProgressState{}, err
	}
	key, err := s.resolveApplyProgressProject(ctx, project)
	if err != nil {
		return ApplyProgressState{}, err
	}
	result, err := s.sdd.GetApplyProgress(key, change)
	return result, withProjectKey(key, err)
}

// GetApplyProgressSuccessorOccupancy is advisory, not a reservation. The
// publisher's transactional decision is authoritative; there is no POST dry-run.
func (s *Service) GetApplyProgressSuccessorOccupancy(ctx context.Context, project, change string) (ApplyProgressSuccessorOccupancy, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return ApplyProgressSuccessorOccupancy{}, ErrProjectRequired
	}
	change, err := validateSDDChange(change)
	if err != nil {
		return ApplyProgressSuccessorOccupancy{}, err
	}
	key, err := s.resolveApplyProgressProject(ctx, project)
	if err != nil {
		return ApplyProgressSuccessorOccupancy{}, err
	}
	result, err := s.sdd.GetApplyProgressSuccessorOccupancy(key, change)
	return result, withProjectKey(key, err)
}

// GetApplyProgressEvidence returns one bounded canonical evidence document only
// when it is referenced by the current resolved guarded snapshot lineage.
func (s *Service) GetApplyProgressEvidence(ctx context.Context, project, change, batchID, expectedHeadDigest string) (applyprogress.Batch, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return applyprogress.Batch{}, ErrProjectRequired
	}
	change, err := validateSDDChange(change)
	if err != nil {
		return applyprogress.Batch{}, err
	}
	key, err := s.resolveApplyProgressProject(ctx, project)
	if err != nil {
		return applyprogress.Batch{}, err
	}
	batch, err := s.sdd.GetApplyProgressEvidence(key, change, batchID, expectedHeadDigest)
	return batch, withProjectKey(key, err)
}

// GetApplyProgressReceipt returns only the identity of a committed request bound
// to one project and change. It never returns the receipt's stored response.
func (s *Service) GetApplyProgressReceipt(ctx context.Context, project, change, requestID string) (ApplyProgressReceipt, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return ApplyProgressReceipt{}, ErrProjectRequired
	}
	change, err := validateSDDChange(change)
	if err != nil {
		return ApplyProgressReceipt{}, err
	}
	key, err := s.resolveApplyProgressProject(ctx, project)
	if err != nil {
		return ApplyProgressReceipt{}, err
	}
	result, err := s.sdd.GetApplyProgressReceipt(key, change, requestID)
	return result, withProjectKey(key, err)
}

// AdvanceApplyProgress performs the dedicated snapshot CAS operation.
func (s *Service) AdvanceApplyProgress(ctx context.Context, request ApplyProgressAdvanceRequest) (ApplyProgressAdvanceResult, error) {
	if strings.TrimSpace(request.Project) == "" {
		return ApplyProgressAdvanceResult{}, ErrProjectRequired
	}
	change, err := validateSDDChange(request.Change)
	if err != nil {
		return ApplyProgressAdvanceResult{}, err
	}
	request.Change = change
	key, err := s.resolveApplyProgressProject(ctx, request.Project)
	if err != nil {
		return ApplyProgressAdvanceResult{}, err
	}
	request.Project = key
	result, err := s.sdd.AdvanceApplyProgress(request)
	return result, withProjectKey(key, err)
}

// PublishApplyProgressSuccessor delegates target derivation and replay authority to the database.
func (s *Service) PublishApplyProgressSuccessor(ctx context.Context, project, predecessorChange string) (ApplyProgressAdvanceResult, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return ApplyProgressAdvanceResult{}, ErrProjectRequired
	}
	change, err := validateSDDChange(predecessorChange)
	if err != nil {
		return ApplyProgressAdvanceResult{}, err
	}
	key, err := s.resolveApplyProgressProject(ctx, project)
	if err != nil {
		return ApplyProgressAdvanceResult{}, err
	}
	result, err := s.sdd.PublishApplyProgressSuccessor(key, change)
	return result, withProjectKey(key, err)
}

func (s *Service) ListSDDChanges(ctx context.Context, request SDDChangePageRequest) (SDDChangePage, error) {
	project := strings.TrimSpace(request.Project)
	if project == "" {
		return SDDChangePage{}, ErrProjectRequired
	}
	if request.Limit < 1 || request.Limit > 100 {
		return SDDChangePage{}, ErrSDDLimitInvalid
	}
	after, err := decodeSDDCursor(request.Cursor)
	if err != nil {
		return SDDChangePage{}, err
	}
	if _, err := s.store.GetGovernanceProject(ctx, project); err != nil {
		return SDDChangePage{}, mapProjectError(err)
	}
	if s.sdd == nil {
		return SDDChangePage{}, errors.New("SDD store is not configured")
	}
	changes, err := s.sdd.ListSDDChanges(project, sddArtifactVocabulary, after, request.Limit+1)
	if err != nil {
		return SDDChangePage{}, err
	}
	page := SDDChangePage{Changes: changes}
	if len(changes) > request.Limit {
		page.Changes = changes[:request.Limit]
		page.NextCursor, err = encodeSDDCursor(page.Changes[len(page.Changes)-1])
		if err != nil {
			return SDDChangePage{}, err
		}
	}
	if page.Changes == nil {
		page.Changes = []string{}
	}
	return page, nil
}

func validateSDDChange(change string) (string, error) {
	change = strings.TrimSpace(change)
	if change == "" {
		return "", ErrSDDChangeRequired
	}
	if strings.ContainsAny(change, `/\\`) || strings.IndexFunc(change, unicode.IsControl) >= 0 {
		return "", ErrSDDChangeInvalid
	}
	return change, nil
}

func encodeSDDCursor(after string) (string, error) {
	payload, err := json.Marshal(sddCursor{After: after})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeSDDCursor(cursor string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", ErrSDDCursorInvalid
	}
	var decoded sddCursor
	if err := json.Unmarshal(payload, &decoded); err != nil || decoded.After == "" {
		return "", ErrSDDCursorInvalid
	}
	canonical, err := json.Marshal(decoded)
	if err != nil || string(canonical) != string(payload) {
		return "", ErrSDDCursorInvalid
	}
	return decoded.After, nil
}
