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
)

type ErrorCode string

const (
	CodeProjectUnknown            ErrorCode = "project_unknown"
	CodeProjectAmbiguous          ErrorCode = "project_ambiguous"
	CodeProjectSessionMismatch    ErrorCode = "project_session_mismatch"
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

	projectName, candidates, err := resolveProject(known, input)
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
	if projectName == "" {
		// The requested project is not in KnownProjects. Check if it has an active
		// alias redirect — source projects are hidden from KnownProjects, so the
		// alias is the recovery path that redirects writes to the target.
		if strings.TrimSpace(input.Project) != "" {
			aliasTarget, found, aliasErr := store.ResolveAlias(ctx, strings.TrimSpace(input.Project))
			if aliasErr != nil {
				return Result{}, fmt.Errorf("resolve alias: %w", aliasErr)
			}
			if found {
				projectName = aliasTarget
			}
		}
		if projectName == "" {
			return Result{}, &ValidationError{Code: CodeProjectUnknown, Message: "project is not known"}
		}
	}

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

func resolveProject(known []KnownProject, input WriteInput) (string, []Candidate, error) {
	if strings.TrimSpace(input.Project) != "" {
		return resolveByName(known, input.Project)
	}
	if strings.TrimSpace(input.Directory) != "" {
		return resolveByDirectory(known, input.Directory)
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
	s = strings.ToLower(strings.TrimSpace(s))
	replacer := strings.NewReplacer("_", "-", " ", "-", ".", "-")
	s = replacer.Replace(s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
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
