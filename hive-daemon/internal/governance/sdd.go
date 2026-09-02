package governance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"unicode"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
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
