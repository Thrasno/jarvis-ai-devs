package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
)

type ErrorCode string

const (
	CodeProjectUnknown            ErrorCode = "project_unknown"
	CodeProjectAmbiguous          ErrorCode = "project_ambiguous"
	CodeProjectSessionMismatch    ErrorCode = "project_session_mismatch"
	CodeProjectIdentityMismatch   ErrorCode = "project_identity_mismatch"
	CodeRecoveryTokenInvalid      ErrorCode = "recovery_token_invalid"
	CodeRecoveryTokenExpired      ErrorCode = "recovery_token_expired"
	CodeRecoveryTokenConsumed     ErrorCode = "recovery_token_consumed"
	CodeRecoveryTokenWrongContext ErrorCode = "recovery_token_wrong_context"
	CodeRecoveryTokenNotCandidate ErrorCode = "recovery_token_not_candidate"
)

var ErrSessionNotFound = errors.New("session not found")

var (
	ErrRecoveryTokenInvalid      = errors.New("recovery token invalid")
	ErrRecoveryTokenExpired      = errors.New("recovery token expired")
	ErrRecoveryTokenConsumed     = errors.New("recovery token consumed")
	ErrRecoveryTokenWrongContext = errors.New("recovery token wrong context")
	ErrRecoveryTokenNotCandidate = errors.New("recovery token not candidate")
)

const DefaultRecoveryTokenTTL = 15 * time.Minute

var defaultRecoveryTokenTTLNanos atomic.Int64

func init() {
	defaultRecoveryTokenTTLNanos.Store(int64(DefaultRecoveryTokenTTL))
}

func ParseRecoveryTokenTTL(value string) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return DefaultRecoveryTokenTTL, nil
	}
	ttl, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse recovery token TTL: %w", err)
	}
	if ttl <= 0 {
		return 0, fmt.Errorf("recovery token TTL must be positive")
	}
	return ttl, nil
}

func SetDefaultRecoveryTokenTTL(ttl time.Duration) {
	if ttl > 0 {
		defaultRecoveryTokenTTLNanos.Store(int64(ttl))
	}
}

type Candidate struct {
	Project   string `json:"project"`
	Directory string `json:"directory,omitempty"`
}

type KnownProject struct {
	Name      string
	Directory string
}

type Store interface {
	KnownProjects(context.Context) ([]KnownProject, error)
	SessionProject(context.Context, string) (string, error)
	CreateRecoveryToken(context.Context, TokenRequest) (string, error)
	ValidateRecoveryToken(context.Context, TokenValidation) error
	ConsumeRecoveryToken(context.Context, TokenValidation) error
	// ResolveAlias returns the target project name if the given name is an active
	// alias source, otherwise returns ("", false, nil).
	ResolveAlias(context.Context, string) (string, bool, error)
}

type WriteInput struct {
	Project             string
	Directory           string
	SessionID           string
	RecoveryToken       string
	ProjectChoiceReason string
}

type Result struct {
	Project string
}

type ValidationError struct {
	Code          ErrorCode   `json:"error_code"`
	Message       string      `json:"error"`
	Candidates    []Candidate `json:"candidates,omitempty"`
	RecoveryToken string      `json:"recovery_token,omitempty"`
	ExpiresAt     time.Time   `json:"expires_at,omitempty"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

type TokenRequest struct {
	Token            string
	Reason           string
	RequestedProject string
	Candidates       []Candidate
	ContextHash      string
	CreatedAt        time.Time
	ExpiresAt        time.Time
}

type TokenValidation struct {
	Token           string
	SelectedProject string
	ContextHash     string
	Now             time.Time
}

type ValidationConfig struct {
	Now      func() time.Time
	TokenTTL time.Duration
}

func ValidateWriteProject(ctx context.Context, store Store, input WriteInput) (Result, error) {
	return ValidateWriteProjectWithConfig(ctx, store, input, ValidationConfig{})
}

func ValidateWriteProjectWithConfig(ctx context.Context, store Store, input WriteInput, cfg ValidationConfig) (Result, error) {
	now := time.Now
	if cfg.Now != nil {
		now = cfg.Now
	}
	ttl := cfg.TokenTTL
	if ttl <= 0 {
		ttl = time.Duration(defaultRecoveryTokenTTLNanos.Load())
	}

	if strings.TrimSpace(input.RecoveryToken) != "" {
		return validateRecoveryRetry(ctx, store, input, now())
	}

	known, err := store.KnownProjects(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("known projects: %w", err)
	}

	projectName, candidates, err := resolveProject(ctx, store, known, input)
	if err != nil {
		return Result{}, err
	}
	if len(candidates) > 1 {
		createdAt := now()
		expiresAt := createdAt.Add(ttl)
		token, err := store.CreateRecoveryToken(ctx, TokenRequest{
			Reason:           string(CodeProjectAmbiguous),
			RequestedProject: input.Project,
			Candidates:       candidates,
			ContextHash:      tokenContextHash(input.Project, input.Directory, input.SessionID),
			CreatedAt:        createdAt,
			ExpiresAt:        expiresAt,
		})
		if err != nil {
			return Result{}, fmt.Errorf("create recovery token: %w", err)
		}
		return Result{}, &ValidationError{Code: CodeProjectAmbiguous, Message: "project resolution is ambiguous", Candidates: candidates, RecoveryToken: token, ExpiresAt: expiresAt}
	}
	if projectName == "" || projectName == "default" {
		return Result{}, &ValidationError{Code: CodeProjectUnknown, Message: "project is not known"}
	}

	// This check deliberately happens after resolution but before the caller is
	// allowed to bootstrap an unknown project. A bound session is stronger
	// evidence than an otherwise corroborated directory identity.
	if err := validateSessionProject(ctx, store, input.SessionID, projectName); err != nil {
		return Result{}, err
	}
	return Result{Project: projectName}, nil
}

func validateRecoveryRetry(ctx context.Context, store Store, input WriteInput, now time.Time) (Result, error) {
	selectedProject := strings.TrimSpace(input.Project)
	if selectedProject == "" {
		return Result{}, &ValidationError{Code: CodeRecoveryTokenNotCandidate, Message: "selected project was not a recovery candidate"}
	}
	contextProject := input.ProjectChoiceReason
	if strings.TrimSpace(contextProject) == "" {
		contextProject = input.Project
	}
	validation := TokenValidation{
		Token:           input.RecoveryToken,
		SelectedProject: selectedProject,
		ContextHash:     tokenContextHash(contextProject, input.Directory, input.SessionID),
		Now:             now,
	}
	if err := store.ValidateRecoveryToken(ctx, validation); err != nil {
		return Result{}, recoveryTokenValidationError(err)
	}
	if err := validateSessionProjectExact(ctx, store, input.SessionID, selectedProject); err != nil {
		return Result{}, err
	}
	if err := store.ConsumeRecoveryToken(ctx, validation); err != nil {
		return Result{}, recoveryTokenValidationError(err)
	}
	return Result{Project: selectedProject}, nil
}

func validateSessionProject(ctx context.Context, store Store, sessionID string, projectName string) error {
	return validateSessionProjectWithMatcher(ctx, store, sessionID, projectName, func(sessionProject, writeProject string) bool {
		return normalizeName(sessionProject) == normalizeName(writeProject)
	})
}

func validateSessionProjectExact(ctx context.Context, store Store, sessionID string, projectName string) error {
	return validateSessionProjectWithMatcher(ctx, store, sessionID, projectName, func(sessionProject, writeProject string) bool {
		return sessionProject == writeProject
	})
}

func validateSessionProjectWithMatcher(ctx context.Context, store Store, sessionID string, projectName string, matches func(string, string) bool) error {
	if sessionID == "" {
		return nil
	}
	sessionProject, err := store.SessionProject(ctx, sessionID)
	if err != nil && !errors.Is(err, ErrSessionNotFound) {
		return fmt.Errorf("session project: %w", err)
	}
	if err == nil && !matches(sessionProject, projectName) {
		return &ValidationError{Code: CodeProjectSessionMismatch, Message: "session project does not match write project", Candidates: []Candidate{{Project: sessionProject}, {Project: projectName}}}
	}
	return nil
}

func recoveryTokenValidationError(err error) error {
	switch {
	case errors.Is(err, ErrRecoveryTokenExpired):
		return &ValidationError{Code: CodeRecoveryTokenExpired, Message: "recovery token expired"}
	case errors.Is(err, ErrRecoveryTokenConsumed):
		return &ValidationError{Code: CodeRecoveryTokenConsumed, Message: "recovery token already consumed"}
	case errors.Is(err, ErrRecoveryTokenWrongContext):
		return &ValidationError{Code: CodeRecoveryTokenWrongContext, Message: "recovery token context does not match request"}
	case errors.Is(err, ErrRecoveryTokenNotCandidate):
		return &ValidationError{Code: CodeRecoveryTokenNotCandidate, Message: "selected project was not a recovery candidate"}
	case errors.Is(err, ErrRecoveryTokenInvalid):
		return &ValidationError{Code: CodeRecoveryTokenInvalid, Message: "recovery token is invalid"}
	default:
		return fmt.Errorf("consume recovery token: %w", err)
	}
}

func resolveProject(ctx context.Context, store Store, known []KnownProject, input WriteInput) (string, []Candidate, error) {
	explicit := strings.TrimSpace(input.Project)
	derived, hasDerivedIdentity, deriveErr := deriveProjectIdentity(input.Directory)

	if explicit != "" {
		if strings.TrimSpace(input.Directory) != "" {
			boundProject, boundCandidates, err := resolveByDirectory(known, input.Directory)
			if err != nil || len(boundCandidates) > 1 {
				return boundProject, boundCandidates, err
			}
			if boundProject != "" {
				if normalizeName(explicit) == normalizeName(boundProject) {
					return boundProject, boundCandidates, nil
				}
				aliasTarget, found, aliasErr := store.ResolveAlias(ctx, explicit)
				if aliasErr != nil {
					return "", nil, fmt.Errorf("resolve alias: %w", aliasErr)
				}
				if found && normalizeName(aliasTarget) == normalizeName(boundProject) {
					return boundProject, boundCandidates, nil
				}
				return "", nil, &ValidationError{
					Code:       CodeProjectIdentityMismatch,
					Message:    "explicit project does not match directory identity",
					Candidates: []Candidate{{Project: explicit}, {Project: boundProject, Directory: input.Directory}},
				}
			}
		}

		projectName, candidates, err := resolveByName(known, explicit)
		if err != nil || len(candidates) > 1 {
			return projectName, candidates, err
		}
		// Without a persisted directory binding, corroborate the caller's requested
		// identity before an alias redirects persistence to its target.
		if hasDerivedIdentity && normalizeName(explicit) != normalizeName(derived) {
			return "", nil, &ValidationError{
				Code:       CodeProjectIdentityMismatch,
				Message:    "explicit project does not match directory identity",
				Candidates: []Candidate{{Project: explicit}, {Project: derived, Directory: input.Directory}},
			}
		}

		if projectName == "" {
			aliasTarget, found, aliasErr := store.ResolveAlias(ctx, explicit)
			if aliasErr != nil {
				return "", nil, fmt.Errorf("resolve alias: %w", aliasErr)
			}
			if found {
				projectName = aliasTarget
			}
		}
		if projectName != "" {
			return projectName, candidates, nil
		}
		if hasDerivedIdentity {
			return derived, nil, nil
		}
		return "", nil, nil
	}

	if strings.TrimSpace(input.Directory) != "" {
		// A registered directory is stronger evidence than a current Git remote or
		// basename. This preserves historical identities after a repository rename.
		projectName, candidates, err := resolveByDirectory(known, input.Directory)
		if err != nil || projectName != "" || len(candidates) > 0 {
			return projectName, candidates, err
		}
	}
	if hasDerivedIdentity {
		projectName, candidates, err := resolveByName(known, derived)
		if err != nil || len(candidates) > 1 {
			return projectName, candidates, err
		}
		if projectName != "" {
			return projectName, candidates, nil
		}
		return derived, nil, nil
	}
	if strings.TrimSpace(input.Directory) != "" && deriveErr != nil {
		return "", nil, &ValidationError{
			Code:    CodeProjectUnknown,
			Message: fmt.Sprintf("project is required: could not derive a project name from directory %q: %v", input.Directory, deriveErr),
		}
	}
	return "", nil, &ValidationError{Code: CodeProjectUnknown, Message: "project is required"}
}

func resolveByName(known []KnownProject, requested string) (string, []Candidate, error) {
	key := normalizeName(requested)
	var matches []Candidate
	for _, knownProject := range known {
		if normalizeName(knownProject.Name) == key {
			matches = append(matches, Candidate{Project: knownProject.Name, Directory: knownProject.Directory})
		}
	}
	unique := uniqueCandidates(matches)
	if len(unique) == 0 {
		return "", nil, nil
	}
	if len(unique) > 1 {
		return "", unique, nil
	}
	return unique[0].Project, unique, nil
}

func resolveByDirectory(known []KnownProject, requested string) (string, []Candidate, error) {
	requestedCanonical := canonicalPath(requested)
	var matches []Candidate
	for _, knownProject := range known {
		if knownProject.Directory == "" {
			continue
		}
		if canonicalPath(knownProject.Directory) == requestedCanonical {
			matches = append(matches, Candidate{Project: knownProject.Name, Directory: knownProject.Directory})
		}
	}
	unique := uniqueCandidates(matches)
	if len(unique) == 0 {
		return "", nil, nil
	}
	if len(unique) > 1 {
		return "", unique, nil
	}
	return unique[0].Project, unique, nil
}

func normalizeName(s string) string {
	return projectidentity.Canonical(s).String()
}

func canonicalPath(path string) string {
	cleaned := filepath.Clean(path)
	if abs, err := filepath.Abs(cleaned); err == nil {
		cleaned = abs
	}
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		cleaned = resolved
	}
	return filepath.Clean(cleaned)
}

func uniqueCandidates(candidates []Candidate) []Candidate {
	seen := map[string]Candidate{}
	for _, candidate := range candidates {
		if candidate.Project == "" {
			continue
		}
		key := candidate.Project + "\x00" + candidate.Directory
		seen[key] = candidate
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]Candidate, 0, len(keys))
	for _, key := range keys {
		result = append(result, seen[key])
	}
	return result
}

func tokenContextHash(projectName, directory, sessionID string) string {
	parts := []string{strings.TrimSpace(projectName), canonicalPath(directory), strings.TrimSpace(sessionID)}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}
