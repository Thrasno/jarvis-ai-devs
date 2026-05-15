package db_test

import (
	"context"
	"testing"

	hivedb "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
)

func TestKnownProjects_ReturnsDistinctProjectsFromWriteTables(t *testing.T) {
	t.Parallel()

	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	if err := d.CreateSession("sess-jarvis", "jarvis-dev", "/repo/jarvis-dev", "dev", "test"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := d.SavePrompt(context.Background(), "prompt-only", "captured prompt"); err != nil {
		t.Fatalf("SavePrompt: %v", err)
	}

	projects, err := d.KnownProjects(context.Background())
	if err != nil {
		t.Fatalf("KnownProjects: %v", err)
	}

	got := map[string]string{}
	for _, p := range projects {
		got[p.Name] = p.Directory
	}
	if got["jarvis-dev"] != "/repo/jarvis-dev" {
		t.Fatalf("jarvis-dev directory = %q, want /repo/jarvis-dev; all=%v", got["jarvis-dev"], got)
	}
	if _, ok := got["prompt-only"]; !ok {
		t.Fatalf("KnownProjects missing prompt-only project from user_prompts; all=%v", got)
	}
}

func TestSessionProject_ReturnsProjectForSession(t *testing.T) {
	t.Parallel()

	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	if err := d.CreateSession("sess-alpha", "alpha", "/repo/alpha", "dev", "test"); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	projectName, err := d.SessionProject(context.Background(), "sess-alpha")
	if err != nil {
		t.Fatalf("SessionProject: %v", err)
	}
	if projectName != "alpha" {
		t.Fatalf("SessionProject = %q, want alpha", projectName)
	}
}
