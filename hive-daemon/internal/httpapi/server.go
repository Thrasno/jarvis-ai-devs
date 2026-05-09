package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/project"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/sanitize"
)

// PromptStore is the minimal interface httpapi needs.
// *db.DB satisfies this via structural typing.
type PromptStore interface {
	SavePrompt(ctx context.Context, project, content string) (*models.Prompt, error)
}

// Server handles HTTP requests for the Hive prompt-capture endpoint.
type Server struct {
	addr     string
	prompts  PromptStore
	projects project.Store
	mux      *http.ServeMux
}

// NewServer constructs a Server bound to addr.
func NewServer(addr string, prompts PromptStore) *Server {
	return NewServerWithProjectStore(addr, prompts, nil)
}

func NewServerWithProjectStore(addr string, prompts PromptStore, projects project.Store) *Server {
	s := &Server{addr: addr, prompts: prompts, projects: projects}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/prompts", s.handlePrompts)
	return s
}

// ServeHTTP implements http.Handler — allows use with httptest.NewRecorder.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// Start launches the HTTP listener as a goroutine and wires ctx cancellation
// to graceful Shutdown. Returns nil on clean shutdown.
func (s *Server) Start(ctx context.Context) error {
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
