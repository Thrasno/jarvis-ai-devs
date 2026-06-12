package governance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/engramimport"
	"github.com/google/uuid"
)

const engramImportPreviewFreshness = 10 * time.Minute

var (
	ErrEngramImportPreviewRequired = errors.New("fresh engram import preview is required")
	ErrEngramImportJobNotFound     = errors.New("engram import job not found")
	ErrEngramImportStoreRequired   = errors.New("engram import store is not configured")
)

type EngramImportJobKind string

const (
	EngramImportJobKindPreview EngramImportJobKind = "preview"
	EngramImportJobKindExecute EngramImportJobKind = "execute"
)

type EngramImportPhase string

const (
	EngramImportPhaseQueued       EngramImportPhase = "queued"
	EngramImportPhaseDiscovery    EngramImportPhase = "discovery"
	EngramImportPhaseAnalysis     EngramImportPhase = "analysis"
	EngramImportPhaseBackup       EngramImportPhase = "backup"
	EngramImportPhaseImport       EngramImportPhase = "import"
	EngramImportPhaseFinalization EngramImportPhase = "finalization"
	EngramImportPhaseCompleted    EngramImportPhase = "completed"
	EngramImportPhaseFailed       EngramImportPhase = "failed"
)

type EngramImportRequest struct {
	Source    string `json:"source,omitempty"`
	PreviewID string `json:"preview_id,omitempty"`
}

type EngramImportEntityCounts struct {
	Sessions     int `json:"sessions"`
	Prompts      int `json:"prompts"`
	Observations int `json:"observations"`
}

type EngramImportMutationCounts struct {
	Imported  int `json:"imported"`
	Reused    int `json:"reused"`
	Ambiguous int `json:"ambiguous"`
}

type EngramImportProjectImpact struct {
	Project   string                   `json:"project"`
	Projected EngramImportEntityCounts `json:"projected"`
}

type EngramImportAmbiguousDuplicate struct {
	SourceID string `json:"source_id"`
	Project  string `json:"project"`
	Title    string `json:"title"`
	Reason   string `json:"reason"`
}

type EngramImportReport struct {
	PreviewID           string                           `json:"preview_id,omitempty"`
	SourcePath          string                           `json:"source_path"`
	SourceFingerprint   string                           `json:"source_fingerprint"`
	Projects            []string                         `json:"projects,omitempty"`
	Projected           EngramImportEntityCounts         `json:"projected"`
	ProjectedByProject  []EngramImportProjectImpact      `json:"projected_by_project,omitempty"`
	Imported            EngramImportMutationCounts       `json:"imported"`
	AmbiguousDuplicates []EngramImportAmbiguousDuplicate `json:"ambiguous_duplicates,omitempty"`
	SkippedRelations    int                              `json:"skipped_relations"`
	InvalidRows         []engramimport.InvalidRow        `json:"invalid_rows,omitempty"`
	BackupID            string                           `json:"backup_id,omitempty"`
}

type EngramImportJob struct {
	ID           string              `json:"id"`
	Kind         EngramImportJobKind `json:"kind"`
	Phase        EngramImportPhase   `json:"phase"`
	Message      string              `json:"message"`
	Processed    int                 `json:"processed"`
	Total        int                 `json:"total"`
	Percent      int                 `json:"percent"`
	Done         bool                `json:"done"`
	Error        string              `json:"error,omitempty"`
	Report       *EngramImportReport `json:"report,omitempty"`
	PhaseHistory []string            `json:"phase_history,omitempty"`
}

type engramImportStore interface {
	ImportEngramBatch(context.Context, db.ImportRun, db.ImportBatch) (db.ImportResult, error)
}

type engramImportPreview struct {
	SourcePath        string
	SourceFingerprint string
	ExpiresAt         time.Time
}

type engramImportJobManager struct {
	mu       sync.Mutex
	jobs     map[string]EngramImportJob
	previews map[string]engramImportPreview
}

func newEngramImportJobManager() *engramImportJobManager {
	return &engramImportJobManager{jobs: map[string]EngramImportJob{}, previews: map[string]engramImportPreview{}}
}

func (s *Service) StartEngramImportPreview(ctx context.Context, req EngramImportRequest) (EngramImportJob, error) {
	if err := ctx.Err(); err != nil {
		return EngramImportJob{}, err
	}
	job := s.createEngramImportJob(EngramImportJobKindPreview)
	go s.runEngramImportPreview(context.Background(), job.ID, req)
	return job, nil
}

func (s *Service) StartEngramImportExecute(ctx context.Context, req EngramImportRequest) (EngramImportJob, error) {
	if err := ctx.Err(); err != nil {
		return EngramImportJob{}, err
	}
	if _, err := s.freshEngramImportPreview(req.PreviewID); err != nil {
		return EngramImportJob{}, err
	}
	job := s.createEngramImportJob(EngramImportJobKindExecute)
	go s.runEngramImportExecute(context.Background(), job.ID, req)
	return job, nil
}

func (s *Service) EngramImportJob(ctx context.Context, id string) (EngramImportJob, error) {
	if err := ctx.Err(); err != nil {
		return EngramImportJob{}, err
	}
	s.ensureEngramImportManager()
	s.imports.mu.Lock()
	defer s.imports.mu.Unlock()
	job, ok := s.imports.jobs[strings.TrimSpace(id)]
	if !ok {
		return EngramImportJob{}, ErrEngramImportJobNotFound
	}
	return job, nil
}

func (s *Service) createEngramImportJob(kind EngramImportJobKind) EngramImportJob {
	s.ensureEngramImportManager()
	job := EngramImportJob{ID: uuid.NewString(), Kind: kind, Phase: EngramImportPhaseQueued, Message: "queued", PhaseHistory: []string{string(EngramImportPhaseQueued)}}
	s.imports.mu.Lock()
	s.imports.jobs[job.ID] = job
	s.imports.mu.Unlock()
	return job
}

func (s *Service) runEngramImportPreview(ctx context.Context, jobID string, req EngramImportRequest) {
	s.updateEngramImportJob(jobID, EngramImportPhaseDiscovery, "resolving Engram source", 0, 0, 5, false, "", nil)
	source, err := resolveEngramImportSource(req)
	if err != nil {
		s.failEngramImportJob(jobID, err)
		return
	}
	s.updateEngramImportJob(jobID, EngramImportPhaseAnalysis, "analyzing Engram source", 0, 0, 35, false, "", nil)
	analysis, err := engramimport.AnalyzeSource(ctx, source)
	if err != nil {
		s.failEngramImportJob(jobID, err)
		return
	}
	report := reportFromAnalysis(jobID, analysis)
	s.recordEngramImportPreview(jobID, analysis.SourcePath, analysis.SourceFingerprint)
	s.updateEngramImportJob(jobID, EngramImportPhaseCompleted, "preview completed", report.Projected.total(), report.Projected.total(), 100, true, "", &report)
}

func (s *Service) runEngramImportExecute(ctx context.Context, jobID string, req EngramImportRequest) {
	preview, err := s.freshEngramImportPreview(req.PreviewID)
	if err != nil {
		s.failEngramImportJob(jobID, err)
		return
	}
	s.updateEngramImportJob(jobID, EngramImportPhaseDiscovery, "resolving Engram source", 0, 0, 5, false, "", nil)
	source, err := resolveEngramImportSource(req)
	if err != nil {
		s.failEngramImportJob(jobID, err)
		return
	}
	s.updateEngramImportJob(jobID, EngramImportPhaseAnalysis, "analyzing Engram source", 0, 0, 25, false, "", nil)
	analysis, err := engramimport.AnalyzeSource(ctx, source)
	if err != nil {
		s.failEngramImportJob(jobID, err)
		return
	}
	if analysis.SourcePath != preview.SourcePath || analysis.SourceFingerprint != preview.SourceFingerprint {
		s.failEngramImportJob(jobID, ErrEngramImportPreviewRequired)
		return
	}
	if s.backup == nil {
		s.failEngramImportJob(jobID, ErrBackupStoreRequired)
		return
	}
	s.updateEngramImportJob(jobID, EngramImportPhaseBackup, "creating Hive backup before import", 0, analysisTotal(analysis), 40, false, "", nil)
	backup, err := s.backup.Create(ctx)
	if err != nil {
		s.failEngramImportJob(jobID, err)
		return
	}
	importer, ok := s.store.(engramImportStore)
	if !ok {
		s.failEngramImportJob(jobID, ErrEngramImportStoreRequired)
		return
	}
	s.updateEngramImportJob(jobID, EngramImportPhaseImport, "importing Engram rows", 0, analysisTotal(analysis), 75, false, "", nil)
	result, err := importer.ImportEngramBatch(ctx, db.ImportRun{ID: jobID, SourceSystem: "engram", SourcePath: analysis.SourcePath, SourceFingerprint: analysis.SourceFingerprint, Mode: "execute"}, engramimport.BuildImportBatch(analysis))
	if err != nil {
		s.failEngramImportJob(jobID, err)
		return
	}
	s.updateEngramImportJob(jobID, EngramImportPhaseFinalization, "finalizing import report", analysisTotal(analysis), analysisTotal(analysis), 95, false, "", nil)
	report := reportFromAnalysis(req.PreviewID, analysis)
	report.BackupID = backup.ID
	report.Imported = EngramImportMutationCounts{Imported: result.Counts.Imported, Reused: result.Counts.Reused, Ambiguous: result.Counts.Ambiguous}
	report.AmbiguousDuplicates = ambiguousDuplicatesReport(result.AmbiguousDuplicates)
	s.updateEngramImportJob(jobID, EngramImportPhaseCompleted, "import completed", analysisTotal(analysis), analysisTotal(analysis), 100, true, "", &report)
}

func (s *Service) updateEngramImportJob(jobID string, phase EngramImportPhase, message string, processed, total, percent int, done bool, errMsg string, report *EngramImportReport) {
	s.ensureEngramImportManager()
	s.imports.mu.Lock()
	defer s.imports.mu.Unlock()
	job := s.imports.jobs[jobID]
	if job.Phase != phase {
		job.PhaseHistory = append(job.PhaseHistory, string(phase))
	}
	job.Phase = phase
	job.Message = message
	job.Processed = processed
	job.Total = total
	job.Percent = clampPercent(percent)
	job.Done = done
	job.Error = errMsg
	job.Report = report
	s.imports.jobs[jobID] = job
}

func (s *Service) failEngramImportJob(jobID string, err error) {
	message := "import job failed"
	if err != nil {
		message = err.Error()
	}
	s.updateEngramImportJob(jobID, EngramImportPhaseFailed, message, 0, 0, 100, true, message, nil)
}

func (s *Service) recordEngramImportPreview(jobID, sourcePath, fingerprint string) {
	s.ensureEngramImportManager()
	s.imports.mu.Lock()
	s.imports.previews[jobID] = engramImportPreview{SourcePath: sourcePath, SourceFingerprint: fingerprint, ExpiresAt: s.currentTime().UTC().Add(engramImportPreviewFreshness)}
	s.imports.mu.Unlock()
}

func (s *Service) freshEngramImportPreview(previewID string) (engramImportPreview, error) {
	s.ensureEngramImportManager()
	s.imports.mu.Lock()
	defer s.imports.mu.Unlock()
	preview, ok := s.imports.previews[strings.TrimSpace(previewID)]
	if !ok || strings.TrimSpace(previewID) == "" || s.currentTime().UTC().After(preview.ExpiresAt) {
		return engramImportPreview{}, ErrEngramImportPreviewRequired
	}
	return preview, nil
}

func (s *Service) ensureEngramImportManager() {
	if s.imports == nil {
		s.imports = newEngramImportJobManager()
	}
}

func resolveEngramImportSource(req EngramImportRequest) (engramimport.Source, error) {
	return engramimport.ResolveSource(engramimport.SourceOptions{ExplicitPath: req.Source, EnvDataDir: os.Getenv("ENGRAM_DATA_DIR")})
}

func reportFromAnalysis(previewID string, analysis engramimport.Analysis) EngramImportReport {
	return EngramImportReport{
		PreviewID:          previewID,
		SourcePath:         analysis.SourcePath,
		SourceFingerprint:  analysis.SourceFingerprint,
		Projects:           append([]string(nil), analysis.Projects...),
		Projected:          EngramImportEntityCounts{Sessions: analysis.Counts.Sessions, Prompts: analysis.Counts.Prompts, Observations: analysis.Counts.Observations},
		ProjectedByProject: projectedByProjectReport(analysis.ProjectedByProject),
		SkippedRelations:   analysis.SkippedRelations,
		InvalidRows:        append([]engramimport.InvalidRow(nil), analysis.InvalidRows...),
	}
}

func projectedByProjectReport(impacts []engramimport.ProjectImpact) []EngramImportProjectImpact {
	report := make([]EngramImportProjectImpact, 0, len(impacts))
	for _, impact := range impacts {
		report = append(report, EngramImportProjectImpact{
			Project:   impact.Project,
			Projected: EngramImportEntityCounts{Sessions: impact.Counts.Sessions, Prompts: impact.Counts.Prompts, Observations: impact.Counts.Observations},
		})
	}
	return report
}

func ambiguousDuplicatesReport(duplicates []db.ImportAmbiguousDuplicate) []EngramImportAmbiguousDuplicate {
	report := make([]EngramImportAmbiguousDuplicate, 0, len(duplicates))
	for _, duplicate := range duplicates {
		report = append(report, EngramImportAmbiguousDuplicate{SourceID: duplicate.SourceID, Project: duplicate.Project, Title: duplicate.Title, Reason: duplicate.Reason})
	}
	return report
}

func analysisTotal(analysis engramimport.Analysis) int {
	return analysis.Counts.Sessions + analysis.Counts.Prompts + analysis.Counts.Observations
}

func (c EngramImportEntityCounts) total() int {
	return c.Sessions + c.Prompts + c.Observations
}

func clampPercent(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func (j EngramImportJob) String() string {
	return fmt.Sprintf("%s %s %s", j.ID, j.Kind, j.Phase)
}
