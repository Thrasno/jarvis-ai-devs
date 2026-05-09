package project

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type ErrorCode string

const (
	CodeProjectUnknown         ErrorCode = "project_unknown"
	CodeProjectAmbiguous       ErrorCode = "project_ambiguous"
	CodeProjectSessionMismatch ErrorCode = "project_session_mismatch"
)

var ErrSessionNotFound = errors.New("session not found")

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
}

type WriteInput struct {
	Project   string
	Directory string
	SessionID string
}

type Result struct {
	Project string
}

type ValidationError struct {
	Code       ErrorCode   `json:"error_code"`
	Message    string      `json:"error"`
	Candidates []Candidate `json:"candidates,omitempty"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func ValidateWriteProject(ctx context.Context, store Store, input WriteInput) (Result, error) {
	known, err := store.KnownProjects(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("known projects: %w", err)
	}

	projectName, candidates, err := resolveProject(known, input)
	if err != nil {
		return Result{}, err
	}
	if len(candidates) > 1 {
		return Result{}, &ValidationError{Code: CodeProjectAmbiguous, Message: "project resolution is ambiguous", Candidates: candidates}
	}
	if projectName == "" {
		return Result{}, &ValidationError{Code: CodeProjectUnknown, Message: "project is not known"}
	}

	if input.SessionID != "" {
		sessionProject, err := store.SessionProject(ctx, input.SessionID)
		if err != nil && !errors.Is(err, ErrSessionNotFound) {
			return Result{}, fmt.Errorf("session project: %w", err)
		}
		if err == nil && normalizeName(sessionProject) != normalizeName(projectName) {
			return Result{}, &ValidationError{Code: CodeProjectSessionMismatch, Message: "session project does not match write project", Candidates: []Candidate{{Project: sessionProject}, {Project: projectName}}}
		}
	}

	return Result{Project: projectName}, nil
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
