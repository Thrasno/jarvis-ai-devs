package db_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	"github.com/stretchr/testify/require"
)

func openPromptWriteDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestSavePromptWithSession_MaterializesAndReopens(t *testing.T) {
	for _, tt := range []struct {
		name  string
		ended bool
	}{
		{name: "absent"},
		{name: "ended", ended: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := openPromptWriteDB(t)
			var before *models.Session
			var err error
			if tt.ended {
				require.NoError(t, d.CreateSession("prompt-session", "alpha", "/original", "first-dev", "first-client"))
				require.NoError(t, d.EndSession("prompt-session", "ended"))
				_, err = d.RawDB().Exec(`UPDATE sessions SET sync_from_project = 'source-project' WHERE id = 'prompt-session'`)
				require.NoError(t, err)
				before, err = d.GetSession("prompt-session")
				require.NoError(t, err)
			}

			prompt, err := d.SavePromptWithSession(context.Background(), models.PromptWrite{
				Session: models.SessionInput{ID: "prompt-session", Project: "Alpha", Directory: "/prompt", DevID: "prompt-dev", Client: "hook"},
				Content: "capture",
			})
			require.NoError(t, err)
			require.Equal(t, "alpha", prompt.Project)
			require.Equal(t, "prompt-session", prompt.SessionID)
			session, err := d.GetSession("prompt-session")
			require.NoError(t, err)
			require.Nil(t, session.EndedAt)
			if tt.ended {
				require.Equal(t, before.SyncID, session.SyncID)
				require.True(t, before.StartedAt.Equal(session.StartedAt))
				require.Equal(t, before.SyncFromProject, session.SyncFromProject)
				require.Equal(t, before.Directory, session.Directory)
				require.Equal(t, before.DevID, session.DevID)
				require.Equal(t, before.Client, session.Client)
			} else {
				require.Equal(t, "hook", session.Client)
				require.Equal(t, "/prompt", session.Directory)
			}
		})
	}
}

func TestSavePromptWithSession_RollsBackSessionAndPrompt(t *testing.T) {
	for _, tt := range []struct {
		name         string
		setup        func(t *testing.T, d *db.DB)
		wantEndedRow bool
	}{
		{
			name: "session insert failure",
			setup: func(t *testing.T, d *db.DB) {
				_, err := d.RawDB().Exec(`CREATE TRIGGER abort_session_insert BEFORE INSERT ON sessions BEGIN SELECT RAISE(ABORT, 'session insert'); END`)
				require.NoError(t, err)
			},
		},
		{
			name: "prompt insert failure after session creation",
			setup: func(t *testing.T, d *db.DB) {
				_, err := d.RawDB().Exec(`CREATE TRIGGER abort_prompt_insert BEFORE INSERT ON user_prompts BEGIN SELECT RAISE(ABORT, 'prompt insert'); END`)
				require.NoError(t, err)
			},
		},
		{
			name: "session reopen failure",
			setup: func(t *testing.T, d *db.DB) {
				require.NoError(t, d.CreateSession("prompt-session", "alpha", "", "dev", "client"))
				require.NoError(t, d.EndSession("prompt-session", "ended"))
				_, err := d.RawDB().Exec(`CREATE TRIGGER abort_session_reopen BEFORE UPDATE OF ended_at ON sessions BEGIN SELECT RAISE(ABORT, 'session reopen'); END`)
				require.NoError(t, err)
			},
			wantEndedRow: true,
		},
		{
			name: "prompt insert failure after ended session reopen",
			setup: func(t *testing.T, d *db.DB) {
				require.NoError(t, d.CreateSession("prompt-session", "alpha", "/original", "dev", "client"))
				require.NoError(t, d.EndSession("prompt-session", "ended"))
				_, err := d.RawDB().Exec(`CREATE TRIGGER abort_prompt_insert BEFORE INSERT ON user_prompts BEGIN SELECT RAISE(ABORT, 'prompt insert'); END`)
				require.NoError(t, err)
			},
			wantEndedRow: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := openPromptWriteDB(t)
			tt.setup(t, d)
			_, err := d.SavePromptWithSession(context.Background(), models.PromptWrite{Session: models.SessionInput{ID: "prompt-session", Project: "alpha"}, Content: "capture"})
			require.Error(t, err)
			var prompts int
			require.NoError(t, d.RawDB().QueryRow(`SELECT COUNT(*) FROM user_prompts`).Scan(&prompts))
			require.Zero(t, prompts)
			session, getErr := d.GetSession("prompt-session")
			if tt.wantEndedRow {
				require.NoError(t, getErr)
				require.NotNil(t, session.EndedAt)
				require.Equal(t, "ended", session.Summary)
			} else {
				require.ErrorIs(t, getErr, db.ErrSessionNotFound)
			}
		})
	}
}

func TestSavePromptWithSession_RejectsMismatchAndBlockedProject(t *testing.T) {
	for _, tt := range []struct {
		name            string
		setup           func(t *testing.T, d *db.DB)
		input           models.SessionInput
		check           func(t *testing.T, err error)
		block           bool
		unwantedProject string
	}{
		{
			name: "mismatch rolls back registered identity and leaves session unchanged",
			setup: func(t *testing.T, d *db.DB) {
				require.NoError(t, d.CreateSession("prompt-session", "alpha", "/original", "dev", "client"))
			},
			input:           models.SessionInput{ID: "prompt-session", Project: "beta", Directory: "/new", Client: "hook"},
			unwantedProject: "beta",
			check: func(t *testing.T, err error) {
				var validation *project.ValidationError
				require.ErrorAs(t, err, &validation)
				require.Equal(t, project.CodeProjectSessionMismatch, validation.Code)
			},
		},
		{
			name: "blocked gate rolls back registered identity and leaves session unchanged",
			setup: func(t *testing.T, d *db.DB) {
				require.NoError(t, d.CreateSession("prompt-session", "alpha", "/original", "dev", "client"))
			},
			input: models.SessionInput{ID: "prompt-session", Project: "alpha", Directory: "/new", Client: "hook"},
			block: true,
			check: func(t *testing.T, err error) {
				require.ErrorIs(t, err, db.ErrProjectBlocked)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := openPromptWriteDB(t)
			tt.setup(t, d)
			before, err := d.GetSession("prompt-session")
			require.NoError(t, err)
			if tt.block {
				_, err = d.RawDB().Exec(`INSERT INTO project_blocks (canonical_project_key, project, command_id, blocked_at) VALUES ('alpha', 'alpha', 'block', CURRENT_TIMESTAMP)`)
				require.NoError(t, err)
			}
			_, err = d.SavePromptWithSession(context.Background(), models.PromptWrite{Session: tt.input, Content: "capture"})
			tt.check(t, err)
			var prompts, identities int
			require.NoError(t, d.RawDB().QueryRow(`SELECT COUNT(*) FROM user_prompts`).Scan(&prompts))
			require.Zero(t, prompts)
			if tt.unwantedProject != "" {
				require.NoError(t, d.RawDB().QueryRow(`SELECT COUNT(*) FROM project_identities WHERE project_key = ?`, tt.unwantedProject).Scan(&identities))
				require.Zero(t, identities)
			}
			if tt.block {
				var projectName, directory, devID, client string
				require.NoError(t, d.RawDB().QueryRow(`SELECT project, directory, dev_id, client FROM sessions WHERE id = ?`, before.ID).Scan(&projectName, &directory, &devID, &client))
				require.Equal(t, before.Project, projectName)
				require.Equal(t, before.Directory, directory)
				require.Equal(t, before.DevID, devID)
				require.Equal(t, before.Client, client)
			} else {
				after, err := d.GetSession("prompt-session")
				require.NoError(t, err)
				require.Equal(t, before, after)
			}
		})
	}
}

func TestSavePromptForSession_PreservesEmptyAndManualBehavior(t *testing.T) {
	d := openPromptWriteDB(t)
	for _, sessionID := range []string{"", "manual-save-alpha"} {
		if _, err := d.SavePromptForSession(context.Background(), "alpha", sessionID, "capture"); err != nil {
			t.Fatal(err)
		}
	}
	var sessions int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 {
		t.Fatalf("sessions = %d, want 0", sessions)
	}
}

func TestSavePromptWithSession_ConcurrentCapturesRemainIndependent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompts.db")
	first, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, d := range []*db.DB{first, second} {
		wg.Add(1)
		go func(d *db.DB) {
			defer wg.Done()
			<-start
			_, err := d.SavePromptWithSession(context.Background(), models.PromptWrite{Session: models.SessionInput{ID: "session", Project: "alpha", Client: "mcp"}, Content: "capture"})
			errs <- err
		}(d)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var prompts int
	if err := first.RawDB().QueryRow(`SELECT COUNT(*) FROM user_prompts`).Scan(&prompts); err != nil {
		t.Fatal(err)
	}
	if prompts != 2 {
		t.Fatalf("prompts = %d, want 2", prompts)
	}
}
