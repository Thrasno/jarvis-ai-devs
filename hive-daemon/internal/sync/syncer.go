package sync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	mathrand "math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
)

const (
	backoffBaseDelay            = 30 * time.Second
	backoffMaxDelay             = 15 * time.Minute
	backoffJitterPct            = 4
	mutationCursorConsumerAPI   = "hive-api"
	compatibilityModeLegacy     = "legacy-row-state"
	compatibilityModeMutationV2 = "mutation-sync-v2"
)

var (
	ErrSyncInFlight = errors.New("sync already in progress")
	ErrSyncBackoff  = errors.New("sync blocked by backoff")
)

// SyncStore define los métodos del DB que necesita el Syncer.
// *db.DB los implementa todos.
type SyncStore interface {
	GetUnsynced(project string) ([]*models.Memory, error)
	MarkSynced(syncID string, at time.Time) error
	MarkMemoriesSyncedBySyncID(syncIDs []string, at time.Time) error
	SaveFromRemote(mem *models.Memory) error
	GetLastSync(project string) (time.Time, error)
	SetLastSync(project string, at time.Time) error
	GetSyncHealth(project string) (db.SyncHealth, error)
	RecordSyncAttempt(project string, at time.Time) error
	RecordSyncSuccess(project string, at time.Time) error
	RecordSyncFailure(project string, at time.Time, consecutiveFailures int, backoffUntil time.Time, syncErr error) error
	GetJWT() string
	SetJWT(token string, expiresAt time.Time) error
	GetUnsyncedPrompts(ctx context.Context, project string) ([]*models.Prompt, error)
	MarkPromptSynced(ctx context.Context, syncID string, at time.Time) error

	// Session sync methods — added in Slice 4 (T4.1 + T4.2).
	ListUnsyncedSessions(project string) ([]*models.Session, error)
	MarkSessionSynced(id string, at time.Time) error
	SaveSessionFromRemote(s *models.Session) error

	GetPendingMutations(project string, limit int) ([]db.MutationEnvelope, error)
	MarkMutationsSynced(eventIDs []string, at time.Time) error
	// MarkMutationsAndMemoriesSynced acks the mutation journal event_ids and
	// the correlated legacy memories.sync_id rows atomically, in a single
	// transaction. The mutation-sync-v2 path in Sync uses this instead of
	// calling MarkMutationsSynced and MarkMemoriesSyncedBySyncID separately,
	// so a partial DB failure rolls back both halves together and the next
	// Sync retries them from a consistent pending state.
	MarkMutationsAndMemoriesSynced(eventIDs []string, syncIDs []string, at time.Time) error
	ApplyRemoteMutation(event db.MutationEnvelope) (bool, error)
	GetMutationCursor(consumer, project string) (db.MutationCursor, error)
	SetMutationCursor(consumer, project string, cursor db.MutationCursor, at time.Time) error
	RecordSyncAttemptLog(ctx context.Context, log db.SyncAttemptLog) error
	ListPendingSyncAttemptLogs(ctx context.Context, limit int) ([]db.SyncAttemptLog, error)
	MarkSyncAttemptLogsDelivered(ctx context.Context, ids []string, at time.Time) error
	DeleteSyncAttemptLogsOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

type BackoffError struct {
	Project string
	RetryAt time.Time
}

func (e *BackoffError) Error() string {
	return fmt.Sprintf("%s: project %s retry at %s", ErrSyncBackoff.Error(), e.Project, e.RetryAt.UTC().Format(time.RFC3339))
}

func (e *BackoffError) Unwrap() error {
	return ErrSyncBackoff
}

// batchResult carries the outcome of a single syncBatchStep push+pull
// exchange (design §4.2, PR 1b-i). It intentionally mirrors the fields Sync
// needs to assemble its final Result and to eventually decide whether the
// Drain controller (PR 1b-ii) should run another batch.
type batchResult struct {
	Pushed            int
	Pulled            int
	MutationsPushed   int
	MutationsPulled   int
	CompatibilityMode string
	MutationCursor    db.MutationCursor

	// PushBacklogEmpty is true when, going into this step, there was no
	// unsynced work of any kind (memories, sessions, prompts, or pending
	// mutations) to send. PR 1b-i only ever runs a single batch step, so the
	// cheapest accurate check is the size of the backlog fetched for this
	// step: if nothing was fetched, nothing remained to send once the step
	// completes. A paged/looping caller (PR 1b-ii Drain controller) can use
	// this to decide whether another push is worth attempting.
	PushBacklogEmpty bool

	// PullHasMore is hardcoded false in PR 1b-i.
	// TODO(PR2): wire from server has_more once hive-api paginates pulls.
	PullHasMore bool
}

// Result resume los resultados de un sync.
type Result struct {
	Pushed            int
	Pulled            int
	Conflicts         int
	PromptsPushed     int
	Project           string
	MutationsPushed   int
	MutationsPulled   int
	CompatibilityMode string
	MutationCursor    db.MutationCursor
}

// Syncer orquesta el ciclo completo de sincronización para un proyecto.
type Syncer struct {
	store    SyncStore
	client   *client
	deps     syncDeps
	mu       sync.Mutex
	inFlight map[string]bool
}

type syncDeps struct {
	now          func() time.Time
	jitter       func(max time.Duration) time.Duration
	newAttemptID func() string
}

// New crea un Syncer con las dependencias inyectadas.
func New(cfg *Config, store SyncStore) *Syncer {
	return newSyncer(cfg, store, defaultSyncDeps())
}

func newTestSyncer(cfg *Config, store SyncStore, deps syncDeps) *Syncer {
	return newSyncer(cfg, store, deps)
}

func newSyncer(cfg *Config, store SyncStore, deps syncDeps) *Syncer {
	if deps.now == nil {
		deps.now = time.Now
	}
	if deps.jitter == nil {
		deps.jitter = defaultSyncDeps().jitter
	}
	if deps.newAttemptID == nil {
		deps.newAttemptID = defaultSyncDeps().newAttemptID
	}
	return &Syncer{
		store:    store,
		client:   newClient(cfg),
		deps:     deps,
		inFlight: make(map[string]bool),
	}
}

func defaultSyncDeps() syncDeps {
	return syncDeps{
		now: time.Now,
		jitter: func(max time.Duration) time.Duration {
			if max <= 0 {
				return 0
			}
			return time.Duration(mathrand.Int63n(int64(max) + 1))
		},
		newAttemptID: newSyncAttemptID,
	}
}

// Sync ejecuta el ciclo completo para un proyecto:
//  1. Obtiene un JWT válido (del caché o haciendo login)
//  2. Obtiene las memorias locales no sincronizadas
//  3. Las envía al servidor (push)
//  4. Recibe las memorias nuevas del servidor (pull)
//  5. Guarda las memorias recibidas localmente
//  6. Actualiza el timestamp de último sync
func (s *Syncer) Sync(ctx context.Context, project string) (*Result, error) {
	if !s.tryStart(project) {
		return nil, fmt.Errorf("%w: project %s", ErrSyncInFlight, project)
	}
	defer s.finish(project)

	health, err := s.store.GetSyncHealth(project)
	if err != nil {
		return nil, fmt.Errorf("obtener estado de sync: %w", err)
	}

	now := s.deps.now().UTC()
	if !health.BackoffUntil.IsZero() && now.Before(health.BackoffUntil) {
		return nil, &BackoffError{Project: project, RetryAt: health.BackoffUntil}
	}

	if err := s.store.RecordSyncAttempt(project, now); err != nil {
		return nil, fmt.Errorf("registrar intento de sync: %w", err)
	}

	// Paso 1: JWT
	token, err := s.getOrRefreshToken(ctx)
	if err != nil {
		if recordErr := s.recordFailure(project, health, now, err); recordErr != nil {
			return nil, recordErr
		}
		return nil, fmt.Errorf("autenticación: %w", err)
	}

	batch, resp, err := s.syncBatchStepWithResponse(ctx, project, token)
	if err != nil {
		// recordFailure must see the raw underlying error (unwrapped), not the
		// "<step label>: %w" wrapper syncBatchStepWithResponse adds — that
		// matches Sync's pre-extraction behavior, where each inline step
		// passed its own raw err to recordFailure and only wrapped the error
		// it returned to its own caller. sanitizeRecordedSyncError (health
		// error text) depends on seeing the raw "sync failed (...)" /
		// "login failed (...)" prefix, so double-wrapping here would corrupt
		// the recorded health.LastError text.
		if recordErr := s.recordFailure(project, health, now, errors.Unwrap(err)); recordErr != nil {
			return nil, recordErr
		}
		return nil, err
	}

	// Paso 6: actualizamos el timestamp del último sync exitoso
	if err := s.store.RecordSyncSuccess(project, now); err != nil {
		return nil, fmt.Errorf("registrar éxito de sync: %w", err)
	}

	attemptLog := db.SyncAttemptLog{
		AttemptID:      s.deps.newAttemptID(),
		DevID:          s.client.cfg.Email,
		Project:        project,
		Client:         "hive-daemon",
		StartedAt:      now,
		EndedAt:        s.deps.now().UTC(),
		Outcome:        db.SyncAttemptOutcomeSuccess,
		HTTPStatus:     httpStatusOK,
		SyncCountsJSON: syncCountsJSON(resp, batch.MutationsPushed, batch.MutationsPulled),
		MetadataJSON:   "{}",
	}
	if err := s.store.RecordSyncAttemptLog(ctx, attemptLog); err != nil {
		logger.Log.Printf("warn: record sync attempt log %s: %v", attemptLog.AttemptID, err)
	} else {
		s.deleteExpiredSyncAttemptLogs(ctx)
		s.flushSyncAttemptLogs(ctx, token)
	}

	return &Result{
		Pushed:            batch.Pushed,
		Pulled:            batch.Pulled,
		Conflicts:         resp.Conflicts,
		PromptsPushed:     resp.PromptsPushed,
		Project:           project,
		MutationsPushed:   batch.MutationsPushed,
		MutationsPulled:   batch.MutationsPulled,
		CompatibilityMode: batch.CompatibilityMode,
		MutationCursor:    batch.MutationCursor,
	}, nil
}

// syncBatchStep runs a single push+pull exchange with the server for
// project: it gathers the locally pending backlog, sends it, applies
// whatever the server pulled back, and acks the backlog it just sent. It
// does NOT touch s.inFlight, the backoff gate, RecordSyncAttempt, or
// RecordSyncSuccess — those remain the caller's responsibility (Sync, for
// now; the Drain controller in PR 1b-ii will call this in a loop instead).
//
// This is a pure extraction of Sync's former push+pull body (design §4.2,
// Hive obs #1692, PR 1b-i) — it must not change observable behavior. See
// TestSyncer_Run and the mutation-protocol-v2 / legacy-fallback tests in
// syncer_test.go for the characterization coverage that pins this contract
// both before and after the extraction.
func (s *Syncer) syncBatchStep(ctx context.Context, project, token string) (batchResult, error) {
	batch, _, err := s.syncBatchStepWithResponse(ctx, project, token)
	return batch, err
}

// syncBatchStepWithResponse is the extraction workhorse: it returns both the
// batchResult (for the Drain controller and for building Result) and the raw
// syncResponse (which Sync still needs for Conflicts/PromptsPushed and to
// build the sync-attempt-log counts JSON). Kept unexported and separate from
// syncBatchStep so the public batch-step signature required by design §4.2
// stays exactly `(batchResult, error)`.
func (s *Syncer) syncBatchStepWithResponse(ctx context.Context, project, token string) (batchResult, *syncResponse, error) {
	// Paso 2: memorias locales pendientes de sync
	unsynced, err := s.store.GetUnsynced(project)
	if err != nil {
		return batchResult{}, nil, fmt.Errorf("obtener memorias no sincronizadas: %w", err)
	}

	// Paso 2b: sesiones locales pendientes de sync (non-fatal si falla)
	unsyncedSessions, err := s.store.ListUnsyncedSessions(project)
	if err != nil {
		logger.Log.Printf("warn: obtener sesiones no sincronizadas: %v", err)
		unsyncedSessions = nil
	}

	// Paso 2c: prompts locales pendientes de sync (non-fatal si falla)
	unsyncedPrompts, err := s.store.GetUnsyncedPrompts(ctx, project)
	if err != nil {
		logger.Log.Printf("warn: obtener prompts no sincronizados: %v", err)
		unsyncedPrompts = nil
	}

	// Paso 2d: mutaciones locales pendientes para protocolo v2.
	pendingMutations, err := s.store.GetPendingMutations(project, 100)
	if err != nil {
		return batchResult{}, nil, fmt.Errorf("obtener mutaciones pendientes: %w", err)
	}

	mutationCursor, err := s.store.GetMutationCursor(mutationCursorConsumerAPI, project)
	if err != nil {
		return batchResult{}, nil, fmt.Errorf("obtener cursor de mutaciones: %w", err)
	}

	// PushBacklogEmpty: nothing fetched above means nothing remained to send
	// for this step. See the batchResult field doc for why this is the
	// cheapest accurate check for a single, non-paged batch step.
	pushBacklogEmpty := len(unsynced) == 0 && len(unsyncedSessions) == 0 &&
		len(unsyncedPrompts) == 0 && len(pendingMutations) == 0

	now := s.deps.now().UTC()

	// Paso 3 + 4: sync bidireccional con el servidor
	lastSync, _ := s.store.GetLastSync(project)
	var lastSyncPtr *time.Time
	if !lastSync.IsZero() {
		lastSyncPtr = &lastSync
	}

	resp, err := s.client.sync(ctx, token, project, unsyncedSessions, unsynced, unsyncedPrompts, lastSyncPtr, pendingMutations, &mutationCursor)
	if err != nil {
		return batchResult{}, nil, fmt.Errorf("sync con servidor: %w", err)
	}

	compatibilityMode := compatibilityModeLegacy
	if resp.CompatibilityMode != "" {
		compatibilityMode = resp.CompatibilityMode
	}

	// Paso 5a: marcamos como sincronizadas las sesiones que enviamos
	for _, sess := range unsyncedSessions {
		if err := s.store.MarkSessionSynced(sess.ID, now); err != nil {
			logger.Log.Printf("warn: MarkSessionSynced %s: %v", sess.ID, err)
		}
	}

	// Paso 5b: marcamos como sincronizadas las memorias legacy solo cuando
	// el servidor confirmó el modo row-state. En v2, hive-api ignora memories[]
	// cuando procesa mutations[], así que ackear filas legacy acá perdería datos.
	if compatibilityMode == compatibilityModeLegacy {
		for _, m := range unsynced {
			if err := s.store.MarkSynced(m.SyncID, now); err != nil {
				// No abortamos — mejor tener datos duplicados que perder el sync
				// En el próximo sync, el servidor los rechazará por sync_id duplicado
				logger.Log.Printf("warn: MarkSynced %s: %v", m.SyncID, err)
			}
		}
	}

	// Paso 5c: marcamos como sincronizados los prompts que enviamos (non-fatal)
	for _, p := range unsyncedPrompts {
		if err := s.store.MarkPromptSynced(ctx, p.SyncID, now); err != nil {
			logger.Log.Printf("warn: MarkPromptSynced %s: %v", p.SyncID, err)
		}
	}

	// Paso 5d: guardamos las sesiones que nos mandó el servidor (BEFORE memories — FK ordering)
	for _, remote := range resp.PulledSessions {
		sess := &models.Session{
			ID:        remote.ID,
			SyncID:    remote.SyncID,
			Project:   remote.Project,
			Directory: remote.Directory,
			DevID:     remote.DevID,
			Client:    remote.Client,
			StartedAt: remote.StartedAt,
			EndedAt:   remote.EndedAt,
		}
		if remote.Summary != nil {
			sess.Summary = *remote.Summary
		}
		// R2-CRIT-3 — non-fatal but observable: log persistence failures so silent
		// drops surface in the daemon log instead of vanishing into discarded errors.
		if err := s.store.SaveSessionFromRemote(sess); err != nil {
			logger.Log.Printf("warn: SaveSessionFromRemote %s: %v", remote.ID, err)
		}
	}

	// Paso 5e: guardamos las memorias que nos mandó el servidor
	for _, remote := range resp.Pulled {
		mem := &models.Memory{
			SyncID:        remote.SyncID,
			Project:       remote.Project,
			TopicKey:      remote.TopicKey,
			Category:      remote.Category,
			Title:         remote.Title,
			Content:       remote.Content,
			Tags:          remote.Tags,
			FilesAffected: remote.FilesAffected,
			CreatedBy:     remote.CreatedBy,
			CreatedAt:     remote.CreatedAt,
			UpdatedAt:     remote.UpdatedAt,
			SessionID:     remote.SessionID,
		}
		if err := s.store.SaveFromRemote(mem); err != nil {
			logger.Log.Printf("warn: SaveFromRemote %s: %v", remote.SyncID, err)
		}
	}

	mutationsPushed := 0
	mutationsPulled := 0
	if resp.CompatibilityMode == compatibilityModeMutationV2 {
		for _, remoteMutation := range resp.PulledMutations {
			applied, err := s.store.ApplyRemoteMutation(remoteMutation)
			if err != nil {
				return batchResult{}, nil, fmt.Errorf("aplicar mutación remota %s: %w", remoteMutation.EventID, err)
			}
			if applied {
				mutationsPulled++
			}
		}

		if resp.NextMutationCursor != nil {
			if err := s.store.SetMutationCursor(mutationCursorConsumerAPI, project, *resp.NextMutationCursor, now); err != nil {
				return batchResult{}, nil, fmt.Errorf("guardar cursor de mutaciones: %w", err)
			}
			mutationCursor = *resp.NextMutationCursor
		}

		// R1a.3 fix (design §3, Hive obs #1692): hive-api ignores memories[]
		// in mutation-sync-v2 mode, so legacy memory rows are never acked by
		// the Paso 5b branch below. Once the pending mutations are durably
		// confirmed, correlate their EntitySyncID back to the legacy
		// memories.sync_id and ack those rows too — otherwise GetUnsynced
		// would keep re-emitting them on every cycle forever.
		//
		// R1a.5 fix (fresh-context review follow-up): the mutation ack and
		// the legacy memory ack MUST happen in the same transaction via
		// MarkMutationsAndMemoriesSynced. Previously these were two separate
		// calls — MarkMutationsSynced then MarkMemoriesSyncedBySyncID — and if
		// the first succeeded but the second failed, the mutation rows were
		// already marked synced_at, so GetPendingMutations would never
		// re-derive confirmedMemorySyncIDs for them again on retry. That left
		// the correlated legacy memories row permanently unsynced. Marking
		// both halves atomically means a partial failure rolls the mutation
		// ack back too, so the next Sync() call re-derives and retries both
		// together from a consistent pending state.
		if err := s.store.MarkMutationsAndMemoriesSynced(
			mutationEventIDs(pendingMutations),
			confirmedMemorySyncIDs(pendingMutations),
			now,
		); err != nil {
			return batchResult{}, nil, fmt.Errorf("marcar mutaciones sincronizadas: %w", err)
		}
		mutationsPushed = len(pendingMutations)
	}

	return batchResult{
		Pushed:            resp.Pushed,
		Pulled:            len(resp.Pulled),
		MutationsPushed:   mutationsPushed,
		MutationsPulled:   mutationsPulled,
		CompatibilityMode: compatibilityMode,
		MutationCursor:    mutationCursor,
		PushBacklogEmpty:  pushBacklogEmpty,
		// TODO(PR2): wire from server has_more once hive-api paginates pulls.
		PullHasMore: false,
	}, resp, nil
}

const syncAttemptFlushLimit = 100
const syncAttemptRetentionDays = 90
const httpStatusOK = 200

func (s *Syncer) flushSyncAttemptLogs(ctx context.Context, token string) {
	pending, err := s.store.ListPendingSyncAttemptLogs(ctx, syncAttemptFlushLimit)
	if err != nil {
		logger.Log.Printf("warn: list pending sync attempt logs: %v", err)
		return
	}
	if len(pending) == 0 {
		return
	}

	response, err := s.client.syncAttempts(ctx, token, pending)
	if err != nil {
		logger.Log.Printf("warn: flush sync attempt logs: %v", err)
		return
	}

	deliveredIDs := append([]string{}, response.AcceptedIDs...)
	deliveredIDs = append(deliveredIDs, response.DuplicateIDs...)
	if len(deliveredIDs) == 0 {
		return
	}
	if err := s.store.MarkSyncAttemptLogsDelivered(ctx, deliveredIDs, s.deps.now().UTC()); err != nil {
		logger.Log.Printf("warn: mark sync attempt logs delivered: %v", err)
	}
}

func (s *Syncer) deleteExpiredSyncAttemptLogs(ctx context.Context) {
	cutoff := s.deps.now().UTC().AddDate(0, 0, -syncAttemptRetentionDays)
	if _, err := s.store.DeleteSyncAttemptLogsOlderThan(ctx, cutoff); err != nil {
		logger.Log.Printf("warn: delete expired sync attempt logs: %v", err)
	}
}

func syncCountsJSON(resp *syncResponse, mutationsPushed, mutationsPulled int) string {
	counts := map[string]int{
		"pushed":           resp.Pushed,
		"pulled":           len(resp.Pulled),
		"conflicts":        resp.Conflicts,
		"prompts_pushed":   resp.PromptsPushed,
		"mutations_pushed": mutationsPushed,
		"mutations_pulled": mutationsPulled,
	}
	encoded, err := json.Marshal(counts)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func newSyncAttemptID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("attempt-%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func mutationEventIDs(mutations []db.MutationEnvelope) []string {
	ids := make([]string, 0, len(mutations))
	for _, mutation := range mutations {
		if mutation.EventID != "" {
			ids = append(ids, mutation.EventID)
		}
	}
	return ids
}

// confirmedMemorySyncIDs returns the legacy memories.sync_id values that can
// be acked once the given mutations are durably confirmed via
// MarkMutationsAndMemoriesSynced. Only create/update mutations carry the
// memory content sync_id that the legacy row shares; delete/restore
// mutations use tombstone semantics and are already acked as part of
// ApplyRemoteMutation/legacy tombstone handling, not via this correlation
// path.
func confirmedMemorySyncIDs(mutations []db.MutationEnvelope) []string {
	ids := make([]string, 0, len(mutations))
	for _, mutation := range mutations {
		// EntityType == "" is treated as "memory" here to mirror the same
		// default applied by db.(*DB).ApplyRemoteMutation (internal/db/sync.go),
		// which normalizes an empty EntityType to "memory" before validating
		// it. Today EntityType is only ever "" or "memory", so both sites
		// agree. If a second entity type is ever introduced, update both
		// call sites together — otherwise "" mutations could silently
		// misclassify as memory mutations here while being rejected or
		// handled differently there.
		if mutation.EntityType != "" && mutation.EntityType != "memory" {
			continue
		}
		switch mutation.Op {
		case db.MutationOpCreate, db.MutationOpUpdate:
		default:
			continue
		}
		if mutation.EntitySyncID != "" {
			ids = append(ids, mutation.EntitySyncID)
		}
	}
	return ids
}

func (s *Syncer) tryStart(project string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inFlight[project] {
		return false
	}
	s.inFlight[project] = true
	return true
}

func (s *Syncer) finish(project string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inFlight, project)
}

func (s *Syncer) recordFailure(project string, health db.SyncHealth, at time.Time, syncErr error) error {
	consecutiveFailures := health.ConsecutiveFailures + 1
	backoffDelay := computeBackoffDelay(consecutiveFailures, s.deps.jitter)
	backoffUntil := at.Add(backoffDelay)
	if err := s.store.RecordSyncFailure(project, at, consecutiveFailures, backoffUntil, syncErr); err != nil {
		return fmt.Errorf("registrar fallo de sync: %w", err)
	}
	s.recordFailureAttemptLog(context.Background(), project, at, syncErr)
	return nil
}

func (s *Syncer) recordFailureAttemptLog(ctx context.Context, project string, startedAt time.Time, syncErr error) {
	message := ""
	if syncErr != nil {
		message = syncErr.Error()
	}
	attemptLog := db.SyncAttemptLog{
		AttemptID:      s.deps.newAttemptID(),
		DevID:          s.client.cfg.Email,
		Project:        project,
		Client:         "hive-daemon",
		StartedAt:      startedAt,
		EndedAt:        s.deps.now().UTC(),
		Outcome:        db.SyncAttemptOutcomeFailure,
		HTTPStatus:     syncAttemptHTTPStatus(message),
		ErrorCode:      "sync_failed",
		ErrorMessage:   message,
		SyncCountsJSON: "{}",
		MetadataJSON:   "{}",
	}
	if err := s.store.RecordSyncAttemptLog(ctx, attemptLog); err != nil {
		logger.Log.Printf("warn: record failed sync attempt log %s: %v", attemptLog.AttemptID, err)
		return
	}
	s.deleteExpiredSyncAttemptLogs(ctx)
}

func syncAttemptHTTPStatus(message string) int {
	start := strings.Index(message, "(")
	end := strings.Index(message, ")")
	if start == -1 || end <= start+1 {
		return 0
	}
	status, err := strconv.Atoi(message[start+1 : end])
	if err != nil {
		return 0
	}
	return status
}

func computeBackoffDelay(consecutiveFailures int, jitter func(max time.Duration) time.Duration) time.Duration {
	if consecutiveFailures <= 0 {
		consecutiveFailures = 1
	}

	delay := backoffBaseDelay
	for attempt := 1; attempt < consecutiveFailures; attempt++ {
		if delay >= backoffMaxDelay {
			break
		}
		delay *= 2
		if delay > backoffMaxDelay {
			delay = backoffMaxDelay
		}
	}

	maxJitter := delay / backoffJitterPct
	if maxJitter <= 0 || jitter == nil {
		return delay
	}

	extra := jitter(maxJitter)
	if extra < 0 {
		extra = 0
	}
	if extra > maxJitter {
		extra = maxJitter
	}
	if delay+extra > backoffMaxDelay {
		return backoffMaxDelay
	}
	return delay + extra
}

// getOrRefreshToken devuelve el JWT cacheado si es válido, o hace login.
func (s *Syncer) getOrRefreshToken(ctx context.Context) (string, error) {
	if token := s.store.GetJWT(); token != "" {
		return token, nil
	}

	token, expiresAt, err := s.client.login(ctx)
	if err != nil {
		return "", err
	}

	_ = s.store.SetJWT(token, expiresAt)
	return token, nil
}
