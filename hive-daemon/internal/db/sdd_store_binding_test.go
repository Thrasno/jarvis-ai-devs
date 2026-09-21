package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSDDStoreBindingFreshSchemaAndMissingReadDoesNotMutate(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	var table string
	require.NoError(t, d.RawDB().QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'sdd_store_bindings'`).Scan(&table))
	require.Equal(t, "sdd_store_bindings", table)

	binding, found, err := d.GetSDDStoreBinding(ctx, " Example.Project ", " change-one ")
	require.NoError(t, err)
	require.False(t, found)
	require.Equal(t, SDDStoreBinding{}, binding)

	var rows int
	require.NoError(t, d.RawDB().QueryRow(`SELECT COUNT(*) FROM sdd_store_bindings`).Scan(&rows))
	require.Zero(t, rows, "a missing read must not create a binding")
	require.NoError(t, d.RawDB().QueryRow(`SELECT COUNT(*) FROM project_identities`).Scan(&rows))
	require.Zero(t, rows, "a missing read must not register project identity state")
}

func TestSDDStoreBindingUpgradeAddsTableAndPreservesUnrelatedDurableData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pre-binding.db")
	seed, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = seed.Exec(`CREATE TABLE project_identities (
		project_key TEXT PRIMARY KEY,
		first_spelling TEXT NOT NULL,
		first_seen_at DATETIME NOT NULL,
		first_source TEXT NOT NULL,
		remote_spelling TEXT NOT NULL DEFAULT '',
		remote_seen_at DATETIME,
		remote_source TEXT NOT NULL DEFAULT ''
	)`)
	require.NoError(t, err)
	_, err = seed.Exec(`INSERT INTO project_identities (project_key, first_spelling, first_seen_at, first_source) VALUES ('legacy-project', 'Legacy Project', CURRENT_TIMESTAMP, 'seed')`)
	require.NoError(t, err)
	require.NoError(t, seed.Close())

	d, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })

	var table, spelling string
	require.NoError(t, d.RawDB().QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'sdd_store_bindings'`).Scan(&table))
	require.Equal(t, "sdd_store_bindings", table)
	require.NoError(t, d.RawDB().QueryRow(`SELECT first_spelling FROM project_identities WHERE project_key = 'legacy-project'`).Scan(&spelling))
	require.Equal(t, "Legacy Project", spelling)
	requireSDDStoreBindingBlankRowsRejected(t, d)
}

func TestSDDStoreBindingPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hive.db")
	ctx := context.Background()
	d, err := Open(path)
	require.NoError(t, err)

	want, created, err := d.AdoptSDDStoreBinding(ctx, "Example.Project", " change-one ", SDDStoreBindingRequest{
		Mode:       SDDStoreModeHybrid,
		Provenance: " migration-test ",
	})
	require.NoError(t, err)
	require.True(t, created)
	// initSchema is the idempotent migration path used by a daemon restart; it
	// must retain the immutable binding rather than recreating or rewriting it.
	require.NoError(t, initSchema(d.RawDB()))
	require.NoError(t, d.Close())

	d, err = Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })

	got, found, err := d.GetSDDStoreBinding(ctx, "example/project", "change-one")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, want, got)
}

func TestSDDStoreBindingRejectsInvalidInputs(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	valid := SDDStoreBindingRequest{Mode: SDDStoreModeHive, Provenance: "explicit"}

	for _, test := range []struct {
		name    string
		project string
		change  string
		request SDDStoreBindingRequest
	}{
		{name: "blank project", project: " ", change: "change", request: valid},
		{name: "blank change", project: "project", change: " ", request: valid},
		{name: "slash in change", project: "project", change: "bad/change", request: valid},
		{name: "backslash in change", project: "project", change: `bad\change`, request: valid},
		{name: "control character in change", project: "project", change: "bad\nchange", request: valid},
		{name: "none mode", project: "project", change: "change", request: SDDStoreBindingRequest{Mode: SDDStoreModeNone, Provenance: "explicit"}},
		{name: "openspec mode", project: "project", change: "change", request: SDDStoreBindingRequest{Mode: SDDStoreModeOpenSpec, Provenance: "explicit"}},
		{name: "unknown mode", project: "project", change: "change", request: SDDStoreBindingRequest{Mode: "unknown", Provenance: "explicit"}},
		{name: "blank provenance", project: "project", change: "change", request: SDDStoreBindingRequest{Mode: SDDStoreModeHive, Provenance: " \t "}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := d.AdoptSDDStoreBinding(ctx, test.project, test.change, test.request)
			require.ErrorIs(t, err, ErrSDDStoreBindingInvalid)
		})
	}

	_, _, err := d.GetSDDStoreBinding(ctx, "project", "bad/change")
	require.ErrorIs(t, err, ErrSDDStoreBindingInvalid)

	var rows int
	require.NoError(t, d.RawDB().QueryRow(`SELECT COUNT(*) FROM sdd_store_bindings`).Scan(&rows))
	require.Zero(t, rows)
}

func TestSDDStoreBindingCanonicalProjectKeyAndExactRead(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	want, created, err := d.AdoptSDDStoreBinding(ctx, " Foo.Bar ", " change-one ", SDDStoreBindingRequest{
		Mode:       SDDStoreModeHive,
		Provenance: " operator selected hive ",
	})
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, "foo-bar", want.Project)
	require.Equal(t, "change-one", want.Change)
	require.Equal(t, SDDStoreBindingSchemaVersion, want.SchemaVersion)
	require.Equal(t, "operator selected hive", want.Provenance)

	got, found, err := d.GetSDDStoreBinding(ctx, "FOO/bar", " change-one ")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, want, got, "reads must return the stored immutable values exactly")

	var rows int
	require.NoError(t, d.RawDB().QueryRow(`SELECT COUNT(*) FROM sdd_store_bindings`).Scan(&rows))
	require.Equal(t, 1, rows, "reads must not mutate or duplicate bindings")
}

func TestSDDStoreBindingSchemaRejectsBlankImmutableColumnsAndReadsFutureVersion(t *testing.T) {
	d := openTestDB(t)
	requireSDDStoreBindingBlankRowsRejected(t, d)

	_, err := d.RawDB().Exec(`INSERT INTO sdd_store_bindings (project, change_name, schema_version, mode, provenance) VALUES ('project', 'change', '2', 'hive', 'future schema')`)
	require.NoError(t, err)
	got, found, err := d.GetSDDStoreBinding(context.Background(), "project", "change")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "2", got.SchemaVersion, "reads must preserve future nonblank schema versions")
	require.Equal(t, "future schema", got.Provenance)
}

func requireSDDStoreBindingBlankRowsRejected(t *testing.T, d *DB) {
	t.Helper()
	for _, test := range []struct {
		name       string
		project    string
		change     string
		schema     string
		provenance string
	}{
		{name: "blank project", project: " \t\n\r", change: "change", schema: "1", provenance: "source"},
		{name: "blank change", project: "project", change: " \t\n\r", schema: "1", provenance: "source"},
		{name: "blank schema", project: "project", change: "change", schema: " \t\n\r", provenance: "source"},
		{name: "blank provenance", project: "project", change: "change", schema: "1", provenance: " \t\n\r"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := d.RawDB().Exec(`INSERT INTO sdd_store_bindings (project, change_name, schema_version, mode, provenance) VALUES (?, ?, ?, 'hive', ?)`, test.project, test.change, test.schema, test.provenance)
			require.Error(t, err)
		})
	}
}

func TestSDDStoreBindingSchemaRejectsAllTrimSpaceWhitespace(t *testing.T) {
	d := openTestDB(t)
	columns := []string{"project", "change", "schema", "provenance"}
	for runeIndex, whitespace := range sddStoreBindingTrimSpaceRunes {
		for _, column := range columns {
			t.Run(fmt.Sprintf("%s/U+%04X", column, whitespace), func(t *testing.T) {
				project := fmt.Sprintf("project-%d-%s", runeIndex, column)
				change := fmt.Sprintf("change-%d-%s", runeIndex, column)
				schema := fmt.Sprintf("schema-%d-%s", runeIndex, column)
				provenance := fmt.Sprintf("provenance-%d-%s", runeIndex, column)
				switch column {
				case "project":
					project = string(whitespace)
				case "change":
					change = string(whitespace)
				case "schema":
					schema = string(whitespace)
				case "provenance":
					provenance = string(whitespace)
				}
				_, err := d.RawDB().Exec(`INSERT INTO sdd_store_bindings (project, change_name, schema_version, mode, provenance) VALUES (?, ?, ?, 'hive', ?)`, project, change, schema, provenance)
				require.Error(t, err)
			})
		}
	}

	_, err := d.RawDB().Exec(`INSERT INTO sdd_store_bindings (project, change_name, schema_version, mode, provenance) VALUES (?, ?, ?, 'hive', ?)`, "project mixed\u00a0text", "change mixed\u00a0text", "schema mixed\u00a0text", "provenance mixed\u00a0text")
	require.NoError(t, err, "mixed real text must remain valid")
}

var sddStoreBindingTrimSpaceRunes = []rune{
	'\t', '\n', '\v', '\f', '\r', ' ',
	'\u0085', '\u00a0', '\u1680',
	'\u2000', '\u2001', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200a',
	'\u2028', '\u2029', '\u202f', '\u205f', '\u3000',
}

func TestSDDStoreBindingExactReplayAndConflictNeverOverwrite(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	originalRequest := SDDStoreBindingRequest{Mode: SDDStoreModeHive, Provenance: "first writer"}

	original, created, err := d.AdoptSDDStoreBinding(ctx, "project", "change", originalRequest)
	require.NoError(t, err)
	require.True(t, created)

	replayed, created, err := d.AdoptSDDStoreBinding(ctx, "project", "change", originalRequest)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, original, replayed)

	requested := SDDStoreBindingRequest{Mode: SDDStoreModeHybrid, Provenance: "different writer"}
	_, created, err = d.AdoptSDDStoreBinding(ctx, "project", "change", requested)
	require.False(t, created)
	var conflict *SDDStoreBindingConflictError
	require.True(t, errors.As(err, &conflict))
	require.Equal(t, original, conflict.Existing)
	require.Equal(t, SDDStoreBinding{Project: "project", Change: "change", SchemaVersion: SDDStoreBindingSchemaVersion, Mode: requested.Mode, Provenance: requested.Provenance}, conflict.Requested)

	_, err = d.RawDB().Exec(`UPDATE sdd_store_bindings SET schema_version = '2' WHERE project = 'project' AND change_name = 'change'`)
	require.NoError(t, err)
	_, created, err = d.AdoptSDDStoreBinding(ctx, "project", "change", originalRequest)
	require.False(t, created)
	require.True(t, errors.As(err, &conflict))
	require.Equal(t, "2", conflict.Existing.SchemaVersion)
	require.Equal(t, SDDStoreBindingSchemaVersion, conflict.Requested.SchemaVersion)

	got, found, err := d.GetSDDStoreBinding(ctx, "project", "change")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "2", got.SchemaVersion, "conflicts must never overwrite an existing binding")
	require.Equal(t, original.Mode, got.Mode)
	require.Equal(t, original.Provenance, got.Provenance)
}

func TestSDDStoreBindingConcurrentDivergentFirstWritersPreserveAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hive.db")
	ctx := context.Background()
	requests := []SDDStoreBindingRequest{
		{Mode: SDDStoreModeHive, Provenance: "writer-a"},
		{Mode: SDDStoreModeHybrid, Provenance: "writer-b"},
		{Mode: SDDStoreModeHive, Provenance: "writer-a"},
		{Mode: SDDStoreModeHybrid, Provenance: "writer-c"},
	}
	stores := make([]*DB, 0, len(requests))
	for range requests {
		store, err := Open(path)
		require.NoError(t, err)
		stores = append(stores, store)
		t.Cleanup(func() { _ = store.Close() })
	}

	type result struct {
		request SDDStoreBindingRequest
		binding SDDStoreBinding
		created bool
		err     error
	}
	start := make(chan struct{})
	results := make(chan result, len(requests))
	var group sync.WaitGroup
	for index, request := range requests {
		group.Add(1)
		go func(d *DB, request SDDStoreBindingRequest) {
			defer group.Done()
			<-start
			binding, created, err := d.AdoptSDDStoreBinding(ctx, "Project", "change", request)
			results <- result{request: request, binding: binding, created: created, err: err}
		}(stores[index], request)
	}
	close(start)
	group.Wait()
	close(results)

	authoritative, found, err := stores[0].GetSDDStoreBinding(ctx, "project", "change")
	require.NoError(t, err)
	require.True(t, found)
	createdCount := 0
	for outcome := range results {
		matchesWinner := outcome.request.Mode == authoritative.Mode && outcome.request.Provenance == authoritative.Provenance
		if outcome.created {
			createdCount++
			require.NoError(t, outcome.err)
			require.Equal(t, authoritative, outcome.binding)
			continue
		}
		if matchesWinner {
			require.NoError(t, outcome.err)
			require.Equal(t, authoritative, outcome.binding)
			continue
		}
		var conflict *SDDStoreBindingConflictError
		require.True(t, errors.As(outcome.err, &conflict))
		require.Equal(t, authoritative, conflict.Existing)
		require.Equal(t, outcome.request.Mode, conflict.Requested.Mode)
		require.Equal(t, outcome.request.Provenance, conflict.Requested.Provenance)
	}
	require.Equal(t, 1, createdCount)

	got, found, err := stores[0].GetSDDStoreBinding(ctx, "project", "change")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, authoritative, got, "losing adoptions must not overwrite the authoritative binding")
}

func TestSDDStoreBindingConcurrentFirstWriterCreatesExactlyOneBinding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hive.db")
	ctx := context.Background()
	request := SDDStoreBindingRequest{Mode: SDDStoreModeHybrid, Provenance: "concurrent first writer"}

	const writers = 8
	stores := make([]*DB, 0, writers)
	for range writers {
		store, err := Open(path)
		require.NoError(t, err)
		stores = append(stores, store)
		t.Cleanup(func() { _ = store.Close() })
	}

	start := make(chan struct{})
	errs := make(chan error, writers)
	created := make(chan bool, writers)
	var group sync.WaitGroup
	for index := range writers {
		group.Add(1)
		go func(d *DB) {
			defer group.Done()
			<-start
			_, didCreate, err := d.AdoptSDDStoreBinding(ctx, "Project", "change", request)
			errs <- err
			created <- didCreate
		}(stores[index])
	}
	close(start)
	group.Wait()
	close(errs)
	close(created)

	createdCount := 0
	for err := range errs {
		require.NoError(t, err)
	}
	for didCreate := range created {
		if didCreate {
			createdCount++
		}
	}
	require.Equal(t, 1, createdCount)

	got, found, err := stores[0].GetSDDStoreBinding(ctx, "project", "change")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, SDDStoreModeHybrid, got.Mode)
	require.Equal(t, "concurrent first writer", got.Provenance)
}
