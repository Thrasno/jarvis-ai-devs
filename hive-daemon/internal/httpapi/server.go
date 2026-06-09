package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/sanitize"
)

const (
	defaultGovernanceMemoryLimit = 100
	maxGovernanceMemoryLimit     = 500
)

// PromptStore is the minimal interface httpapi needs.
// *db.DB satisfies this via structural typing.
type PromptStore interface {
	SavePrompt(ctx context.Context, project, content string) (*models.Prompt, error)
}

type GovernanceService interface {
	Projects(context.Context) ([]governance.Project, error)
	Project(context.Context, string) (governance.Project, error)
	Memories(context.Context, governance.MemoryFilter) ([]governance.Memory, error)
	MemoryByID(context.Context, int64) (governance.Memory, error)
	Health(context.Context) ([]governance.Health, error)
	Warnings(context.Context, governance.WarningFilter) ([]governance.Warning, error)
	Backups(context.Context) ([]governance.BackupManifest, error)
	CreateBackup(context.Context) (governance.BackupManifest, error)
	RestoreBackup(context.Context, governance.RestoreRequest) (governance.RestoreResult, error)
	ExecuteGuard(context.Context, governance.GuardRequest) (governance.GuardResult, error)
	ExecuteProjectArchive(context.Context, governance.ProjectArchiveRequest) (governance.ProjectArchiveResult, error)
	ExecuteProjectMerge(context.Context, governance.ProjectMergeRequest) (governance.ProjectMergeResult, error)
}

// Server handles HTTP requests for the Hive prompt-capture endpoint.
type Server struct {
	addr       string
	prompts    PromptStore
	projects   project.Store
	governance GovernanceService
	mux        *http.ServeMux
}

// NewServer constructs a Server bound to addr.
func NewServer(addr string, prompts PromptStore) *Server {
	return NewServerWithProjectStore(addr, prompts, nil)
}

func NewServerWithProjectStore(addr string, prompts PromptStore, projects project.Store) *Server {
	return NewServerWithProjectStoreAndGovernance(addr, prompts, projects, nil)
}

func NewServerWithGovernance(addr string, prompts PromptStore, governance GovernanceService) *Server {
	return NewServerWithProjectStoreAndGovernance(addr, prompts, nil, governance)
}

func NewServerWithProjectStoreAndGovernance(addr string, prompts PromptStore, projects project.Store, governance GovernanceService) *Server {
	s := &Server{addr: addr, prompts: prompts, projects: projects, governance: governance}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/prompts", s.handlePrompts)
	if governance != nil {
		s.mux.HandleFunc("/governance/projects", s.handleGovernanceProjects)
		s.mux.HandleFunc("/governance/projects/", s.handleGovernanceProject)
		s.mux.HandleFunc("/governance/memories", s.handleGovernanceMemories)
		s.mux.HandleFunc("GET /governance/memories/{id}", s.handleGovernanceMemory)
		s.mux.HandleFunc("/governance/health", s.handleGovernanceHealth)
		s.mux.HandleFunc("/governance/warnings", s.handleGovernanceWarnings)
		s.mux.HandleFunc("/governance/backups", s.handleGovernanceBackups)
		s.mux.HandleFunc("/governance/restores", s.handleGovernanceRestores)
		s.mux.HandleFunc("/governance/guards/execute", s.handleGovernanceGuardExecute)
	}
	return s
}

// ServeHTTP implements http.Handler — allows use with httptest.NewRecorder.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// Start launches the HTTP listener as a goroutine and wires ctx cancellation
// to graceful Shutdown. Returns nil on clean shutdown.
func (s *Server) Start(ctx context.Context) error {
	if !isLoopbackAddr(s.addr) {
		return fmt.Errorf("http server requires a loopback address: %s", s.addr)
	}

	srv := &http.Server{
		Addr:              s.addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Log.Printf("http server listening on %s", s.addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("http listen: %w", err)
	}
}

// Governance HTTP is intentionally local-only for this phase: callers reach the
// daemon through a loopback listener rather than a bearer-token boundary.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || strings.TrimSpace(host) == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) handlePrompts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
	var body struct {
		Content             string `json:"content"`
		Project             string `json:"project"`
		RecoveryToken       string `json:"recovery_token"`
		ProjectChoiceReason string `json:"project_choice_reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "content too large"})
		} else {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid json"})
		}
		return
	}

	if strings.TrimSpace(body.Content) == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "content is required"})
		return
	}

	if strings.TrimSpace(body.Project) == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "project is required"})
		return
	}

	const maxPromptRunes = 50_000
	if utf8.RuneCountInString(body.Content) > maxPromptRunes {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "content too large"})
		return
	}

	if s.projects != nil {
		resolved, err := project.ValidateWriteProject(r.Context(), s.projects, project.WriteInput{Project: body.Project, RecoveryToken: body.RecoveryToken, ProjectChoiceReason: body.ProjectChoiceReason})
		if err != nil {
			writeProjectValidationError(w, err)
			return
		}
		body.Project = resolved.Project
	}

	// Strip private tags from content at the handler boundary.
	contentRes := sanitize.Strip(body.Content)

	prompt, err := s.prompts.SavePrompt(r.Context(), body.Project, contentRes.Clean)
	if err != nil {
		logger.Log.Printf("save prompt: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":             prompt.ID,
		"created_at":     prompt.CreatedAt.Format(time.RFC3339),
		"stripped":       contentRes.Count > 0,
		"stripped_count": contentRes.Count,
	})
}

func writeProjectValidationError(w http.ResponseWriter, err error) {
	var validationErr *project.ValidationError
	if !errors.As(err, &validationErr) {
		logger.Log.Printf("validate project: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(validationErr)
}

func (s *Server) handleGovernanceProjects(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	projects, err := s.governance.Projects(r.Context())
	if err != nil {
		writeInternalError(w, "governance projects", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (s *Server) handleGovernanceProject(w http.ResponseWriter, r *http.Request) {
	escapedName := strings.TrimPrefix(r.URL.EscapedPath(), "/governance/projects/")
	if mergeParts := strings.SplitN(escapedName, "/merge/", 2); len(mergeParts) == 2 {
		sourceProject, err := url.PathUnescape(mergeParts[0])
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source project is invalid"})
			return
		}
		targetProject, err := url.PathUnescape(mergeParts[1])
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target project is invalid"})
			return
		}
		s.handleGovernanceProjectMerge(w, r, sourceProject, targetProject)
		return
	}
	if strings.HasSuffix(escapedName, "/archive") {
		projectName, err := url.PathUnescape(strings.TrimSuffix(escapedName, "/archive"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project is invalid"})
			return
		}
		s.handleGovernanceProjectArchive(w, r, projectName)
		return
	}
	name, err := url.PathUnescape(escapedName)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project is invalid"})
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	project, err := s.governance.Project(r.Context(), name)
	if err != nil {
		writeGovernanceError(w, "governance project", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": project})
}

func (s *Server) handleGovernanceProjectArchive(w http.ResponseWriter, r *http.Request, projectName string) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var body governance.ProjectArchiveRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	body.Project = projectName
	result, err := s.governance.ExecuteProjectArchive(r.Context(), body)
	if err != nil {
		writeGuardError(w, "governance project archive", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func (s *Server) handleGovernanceProjectMerge(w http.ResponseWriter, r *http.Request, sourceProject, targetProject string) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var body governance.ProjectMergeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	body.SourceProject = sourceProject
	body.TargetProject = targetProject
	result, err := s.governance.ExecuteProjectMerge(r.Context(), body)
	if err != nil {
		writeGuardError(w, "governance project merge", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func (s *Server) handleGovernanceMemories(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	limit, err := parseGovernanceMemoryLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	includeDeleted, err := parseOptionalBool(r.URL.Query().Get("include_deleted"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	memories, err := s.governance.Memories(r.Context(), governance.MemoryFilter{
		Project:        r.URL.Query().Get("project"),
		IncludeDeleted: includeDeleted,
		Limit:          limit,
	})
	if err != nil {
		writeGovernanceError(w, "governance memories", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"memories": memories})
}

func (s *Server) handleGovernanceMemory(w http.ResponseWriter, r *http.Request) {
	idRaw := r.PathValue("id")
	id, err := strconv.ParseInt(strings.TrimSpace(idRaw), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "memory id must be a positive integer"})
		return
	}
	memory, err := s.governance.MemoryByID(r.Context(), id)
	if err != nil {
		writeMemoryError(w, "governance memory", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"memory": memory})
}

func (s *Server) handleGovernanceHealth(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	health, err := s.governance.Health(r.Context())
	if err != nil {
		writeInternalError(w, "governance health", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": health})
}

func (s *Server) handleGovernanceWarnings(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	warnings, err := s.governance.Warnings(r.Context(), governance.WarningFilter{
		ResolutionState: r.URL.Query().Get("resolution_state"),
	})
	if err != nil {
		writeInternalError(w, "governance warnings", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"warnings": warnings})
}

func (s *Server) handleGovernanceBackups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		backups, err := s.governance.Backups(r.Context())
		if err != nil {
			writeBackupError(w, "governance backups", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"backups": backups})
	case http.MethodPost:
		backup, err := s.governance.CreateBackup(r.Context())
		if err != nil {
			writeBackupError(w, "governance backup create", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"backup": backup})
	default:
		writeMethodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleGovernanceRestores(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var body governance.RestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	restore, err := s.governance.RestoreBackup(r.Context(), body)
	if err != nil {
		writeBackupError(w, "governance restore", err)
		return
	}
	status := http.StatusOK
	if restore.Status == governance.RestoreStatusCoordinationRequired {
		status = http.StatusAccepted
	}
	writeJSON(w, status, map[string]any{"restore": restore})
}

func (s *Server) handleGovernanceGuardExecute(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var body governance.GuardRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	result, err := s.governance.ExecuteGuard(r.Context(), body)
	if err != nil {
		writeGuardError(w, "governance guard execute", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == method {
		return true
	}
	writeMethodNotAllowed(w, method)
	return false
}

func writeMethodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Allow", strings.Join(methods, ", "))
	w.WriteHeader(http.StatusMethodNotAllowed)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeInternalError(w http.ResponseWriter, source string, err error) {
	logger.Log.Printf("%s: %v", source, err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
}

func writeGovernanceError(w http.ResponseWriter, source string, err error) {
	status := http.StatusInternalServerError
	errorMessage := "internal error"
	if errors.Is(err, governance.ErrProjectRequired) {
		status = http.StatusBadRequest
		errorMessage = "project is required"
	} else if errors.Is(err, governance.ErrProjectNotFound) {
		status = http.StatusNotFound
		errorMessage = "project not found"
	} else {
		logger.Log.Printf("%s: %v", source, err)
	}
	writeJSON(w, status, map[string]string{"error": errorMessage})
}

func writeMemoryError(w http.ResponseWriter, source string, err error) {
	status := http.StatusInternalServerError
	errorMessage := "internal error"
	switch {
	case errors.Is(err, governance.ErrMemoryIDRequired):
		status = http.StatusBadRequest
		errorMessage = "memory id is required"
	case errors.Is(err, governance.ErrMemoryNotFound):
		status = http.StatusNotFound
		errorMessage = "memory not found"
	default:
		logger.Log.Printf("%s: %v", source, err)
	}
	writeJSON(w, status, map[string]string{"error": errorMessage})
}

func writeBackupError(w http.ResponseWriter, source string, err error) {
	status := http.StatusInternalServerError
	errorMessage := "internal error"
	switch {
	case errors.Is(err, governance.ErrBackupStoreRequired):
		status = http.StatusServiceUnavailable
		errorMessage = "backup store is not configured"
	case errors.Is(err, governance.ErrBackupIDRequired):
		status = http.StatusBadRequest
		errorMessage = "backup_id is required"
	case errors.Is(err, governance.ErrBackupConfirmationRequired):
		status = http.StatusBadRequest
		errorMessage = "confirmation is required"
	case errors.Is(err, governance.ErrBackupConfirmationMismatch):
		status = http.StatusBadRequest
		errorMessage = "confirmation mismatch"
	case errors.Is(err, governance.ErrBackupNotFound):
		status = http.StatusNotFound
		errorMessage = "backup not found"
	case errors.Is(err, governance.ErrBackupIDUnsafe):
		status = http.StatusBadRequest
		errorMessage = "backup_id is invalid"
	case errors.Is(err, governance.ErrBackupLocationUnsafe):
		status = http.StatusBadRequest
		errorMessage = "backup root must be outside the live database directory"
	case errors.Is(err, governance.ErrBackupArchiveInvalid):
		status = http.StatusConflict
		errorMessage = "backup archive integrity check failed"
	default:
		logger.Log.Printf("%s: %v", source, err)
	}
	writeJSON(w, status, map[string]string{"error": errorMessage})
}

func writeGuardError(w http.ResponseWriter, source string, err error) {
	status := http.StatusInternalServerError
	errorMessage := "internal error"
	switch {
	case errors.Is(err, governance.ErrDestructiveBackupRequired):
		status = http.StatusBadRequest
		errorMessage = "fresh backup is required before destructive operation"
	case errors.Is(err, governance.ErrProjectRequired):
		status = http.StatusBadRequest
		errorMessage = "project is required"
	case errors.Is(err, governance.ErrProjectNotFound):
		status = http.StatusNotFound
		errorMessage = "project not found"
	case errors.Is(err, governance.ErrBackupArchiveInvalid):
		status = http.StatusConflict
		errorMessage = "backup archive integrity check failed"
	case errors.Is(err, governance.ErrDestructiveConfirmationRequired):
		status = http.StatusBadRequest
		errorMessage = "confirmation is required"
	case errors.Is(err, governance.ErrDestructiveConfirmationMismatch):
		status = http.StatusBadRequest
		errorMessage = "confirmation mismatch"
	case errors.Is(err, governance.ErrDestructiveReasonRequired):
		status = http.StatusBadRequest
		errorMessage = "delete reason is required"
	case errors.Is(err, governance.ErrDestructiveTargetRequired):
		status = http.StatusBadRequest
		errorMessage = "target is required"
	case errors.Is(err, governance.ErrDestructiveOperationUnsupported):
		status = http.StatusBadRequest
		errorMessage = "destructive operation is unsupported"
	case errors.Is(err, governance.ErrBackupStoreRequired), errors.Is(err, governance.ErrDestructiveMutationStoreRequired):
		status = http.StatusServiceUnavailable
		errorMessage = "destructive operation guard is not configured"
	case errors.Is(err, db.ErrMemoryNotFound):
		status = http.StatusNotFound
		errorMessage = "memory not found"
	case errors.Is(err, db.ErrMemoryAlreadyDeleted):
		status = http.StatusConflict
		errorMessage = "memory already deleted"
	case errors.Is(err, db.ErrMemoryNotDeleted):
		status = http.StatusConflict
		errorMessage = "memory is not deleted"
	case errors.Is(err, db.ErrGovernanceProjectMergeInvalid):
		status = http.StatusBadRequest
		errorMessage = "project merge source and target must differ"
	case errors.Is(err, db.ErrGovernanceProjectArchived):
		status = http.StatusConflict
		errorMessage = "project is archived"
	case errors.Is(err, db.ErrGovernanceProjectMergeConflict):
		status = http.StatusConflict
		errorMessage = "project merge conflicts with existing local project governance metadata"
	default:
		logger.Log.Printf("%s: %v", source, err)
	}
	writeJSON(w, status, map[string]string{"error": errorMessage})
}

func parseGovernanceMemoryLimit(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return defaultGovernanceMemoryLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("limit must be an integer")
	}
	if limit <= 0 {
		return defaultGovernanceMemoryLimit, nil
	}
	if limit > maxGovernanceMemoryLimit {
		return 0, fmt.Errorf("limit must be <= %d", maxGovernanceMemoryLimit)
	}
	return limit, nil
}

func parseOptionalBool(raw string) (bool, error) {
	if strings.TrimSpace(raw) == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("include_deleted must be a boolean")
	}
	return value, nil
}
