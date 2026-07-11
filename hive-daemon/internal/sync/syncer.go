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

	// pullCursorChannelMemories/pullCursorChannelSessions identify the two
	// independently-paginated legacy pull channels for GetPullCursor/
	// SetPullCursor (PR 2a/2b, hive-sync-batched-drain).
	pullCursorChannelMemories = "memories"
	pullCursorChannelSessions = "sessions"
)

// syncPageSize caps how many unsynced memories, sessions, and prompts a
// single syncBatchStep fetches and pushes (PR 1b-iii, hive-sync-batched-drain
// design §4.4). It is a package-level var (not a const) so tests can shrink
// it to exercise multi-batch paging without needing hundreds of rows.
// GetPendingMutations already took an explicit limit before this PR; it now
// shares the same page size.
var syncPageSize = 100

// maxDrainBatches bounds the total number of syncBatchStep iterations a
// single Drain(TriggerManual) run may execute (PR 2b infinite-loop fix,
// hive-sync-batched-drain). It is defense-in-depth on top of the pull-cursor
// no-progress guard below: even if that guard's logic somehow has a gap, the
// loop still cannot spin forever. At syncPageSize=100 per channel, a backlog
// at the ~1.5k-5k target scale drains in well under 100 batches per channel,
// so this cap is generous for any legitimate drain while still being a
// small, finite number. It is a package-level var (not a const) so tests can
// shrink it to hit the cap deterministically without looping thousands of
// times.
var maxDrainBatches = 5000

var (
	ErrSyncInFlight = errors.New("sync already in progress")
	ErrSyncBackoff  = errors.New("sync blocked by backoff")
	// ErrStoredCredentialsRejected is safe to return and tells operators how to recover.
	ErrStoredCredentialsRejected = errors.New("Hive API rejected the stored credentials after invalidating the cached session. Open Jarvis → Hive API Config, enter the current account password, save, and restart hive-daemon. No further authentication attempts will run until restart.")
	// ErrCachedSessionCleanupFailed is intentionally sanitized: database errors
	// must not leak through the authentication boundary.
	ErrCachedSessionCleanupFailed = errors.New("could not clear cached Hive API session after rejected credentials; resolve local storage and retry")
	// ErrAuthReloginFailed marks a transient recovery-login failure without
	// retaining credentials or response details.
	ErrAuthReloginFailed = errors.New("Hive API re-authentication failed; retry after backoff")
	// ErrReauthenticatedTokenRejected is terminal for this process: a newly
	// issued token was rejected, so the daemon must not claim stale credentials.
	ErrReauthenticatedTokenRejected = errors.New("Hive API rejected a newly authenticated session. Restart hive-daemon before retrying authentication.")
)

// SyncStore define los métodos del DB que necesita el Syncer.
// *db.DB los implementa todos.
type SyncStore interface {
	GetUnsynced(project string) ([]*models.Memory, error)
	// GetUnsyncedPage is the paged counterpart to GetUnsynced (PR 1b-iii,
	// hive-sync-batched-drain): syncBatchStep uses it to cap a single push
	// batch at syncPageSize instead of always fetching the full backlog.
	// GetUnsynced is kept in the interface for any other existing/future
	// caller that needs the unbounded list.
	GetUnsyncedPage(project string, limit int) ([]*models.Memory, error)
	MarkSynced(syncID string, at time.Time) error
	MarkMemoriesSyncedBySyncID(syncIDs []string, at time.Time) error
	SaveFromRemote(mem *models.Memory) error
	GetLastSync(project string) (time.Time, error)
	SetLastSync(project string, at time.Time) error
	GetSyncHealth(project string) (db.SyncHealth, error)
	RecordSyncAttempt(project string, at time.Time) error
	RecordSyncSuccess(project string, at time.Time) error
	RecordSyncFailure(project string, at time.Time, consecutiveFailures int, backoffUntil time.Time, syncErr error) error
	// RecordDrainOutcome persists the most recently recorded Drain outcome
	// (PR 3, task 3.4, hive-sync-batched-drain) — see db.(*DB).RecordDrainOutcome
	// doc. Drain calls this at the end of every run (success and degraded
	// paths) so the sync health surfaces (mem_sync, health DTO) can report the
	// last drain state without re-deriving it.
	RecordDrainOutcome(project, state, reason string, remaining int) error
	GetJWT() string
	SetJWT(token string, expiresAt time.Time) error
	ClearJWT() error
	GetUnsyncedPrompts(ctx context.Context, project string) ([]*models.Prompt, error)
	// GetUnsyncedPromptsPage is the paged counterpart to GetUnsyncedPrompts
	// (PR 1b-iii, hive-sync-batched-drain) — see GetUnsyncedPage doc.
	GetUnsyncedPromptsPage(ctx context.Context, project string, limit int) ([]*models.Prompt, error)
	MarkPromptSynced(ctx context.Context, syncID string, at time.Time) error

	// Session sync methods — added in Slice 4 (T4.1 + T4.2).
	ListUnsyncedSessions(project string) ([]*models.Session, error)
	// ListUnsyncedSessionsPage is the paged counterpart to ListUnsyncedSessions
	// (PR 1b-iii, hive-sync-batched-drain) — see GetUnsyncedPage doc.
	ListUnsyncedSessionsPage(project string, limit int) ([]*models.Session, error)
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
	// GetPullCursor/SetPullCursor persist the bounded legacy-pull resume
	// position (PR 2a/2b, hive-sync-batched-drain) per (consumer, project,
	// channel). channel is pullCursorChannelMemories or
	// pullCursorChannelSessions — the two legacy pull channels paginate
	// independently.
	GetPullCursor(consumer, project, channel string) (db.PullCursor, error)
	SetPullCursor(consumer, project, channel string, cursor db.PullCursor, at time.Time) error
	ClearPullCursor(consumer, project, channel string) error
	RecordSyncAttemptLog(ctx context.Context, log db.SyncAttemptLog) error
	ListPendingSyncAttemptLogs(ctx context.Context, limit int) ([]db.SyncAttemptLog, error)
	MarkSyncAttemptLogsDelivered(ctx context.Context, ids []string, at time.Time) error
	DeleteSyncAttemptLogsOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
	RecordProjectBlock(ctx context.Context, cmd db.ProjectBlockCommand) (db.ProjectBlock, error)
	QuarantineBlockedProject(ctx context.Context, projectName, actorID, reason string, at time.Time) (db.ProjectQuarantineResult, error)
	RecordPendingProjectBlockAck(ctx context.Context, ack db.ProjectBlockAck) error
	RecordProjectBlockAck(ctx context.Context, ack db.ProjectBlockAck) (db.ProjectBlockAck, error)
	ListPendingProjectBlockAcks(ctx context.Context, limit int) ([]db.ProjectBlockAck, error)
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

	// BacklogSize is the total count of pending items fetched at the start of
	// this step (memories + sessions + prompts + pending mutations). PR 1b-i
	// used it as the Drain progress proxy, but PR 1b-iii caps each fetch at
	// syncPageSize: once the push is paged, BacklogSize saturates at the page
	// size (e.g. always ~100 while draining a 250-item backlog) and stops
	// reflecting whether the batch actually accomplished anything. It is kept
	// here for diagnostics/PushBacklogEmpty only — the Drain no-progress guard
	// must use RecordsMarkedSynced instead (see below).
	BacklogSize int

	// RecordsMarkedSynced is the count of records durably marked synced_at
	// during THIS batch: memories (legacy-mode MarkSynced or v2
	// MarkMutationsAndMemoriesSynced), sessions (MarkSessionSynced), prompts
	// (MarkPromptSynced), and pending mutations (mutationsPushed) that the
	// server actually acknowledged. This is the PR 1b-iii progress signal for
	// the Drain no-progress guard (see Drain): a paged fetch can legitimately
	// return the same page size on every call while the backlog shrinks
	// underneath it, so page size alone can no longer prove progress.
	// RecordsMarkedSynced > 0 means real work was durably confirmed and
	// removed from the backlog this batch, which is a page-size-independent
	// progress signal.
	RecordsMarkedSynced int

	// PullHasMore is hardcoded false in PR 1b-i.
	// TODO(PR2): wire from server has_more once hive-api paginates pulls.
	PullHasMore bool

	// PullMemoriesCursor/PullSessionsCursor (PR 2b infinite-loop fix) are the
	// per-channel pull cursors AFTER this batch: the value the server
	// returned via NextPullCursor/NextSessionCursor, or — when the server
	// did not return one (already-drained channel, or a server that predates
	// pagination) — the cursor that was already persisted before this batch
	// ran. Drain compares these against the previous iteration's values to
	// decide whether the pull side actually advanced, instead of trusting
	// PullHasMore alone (see the Drain no-progress guard doc).
	PullMemoriesCursor db.PullCursor
	PullSessionsCursor db.PullCursor
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
	store                          SyncStore
	client                         *client
	deps                           syncDeps
	mu                             sync.Mutex
	inFlight                       map[string]bool
	authMu                         sync.Mutex
	credentialsStale               bool
	staleDiagnosticEmitted         bool
	reauthenticatedTokenRejected   bool
	tokenRejectedDiagnosticEmitted bool
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

// TriggerPolicy selects how many batch steps a Drain call runs before
// returning (design §2.1, §4.3, PR 1b-ii).
type TriggerPolicy int

const (
	// TriggerAuto runs exactly one syncBatchStep, matching the historical
	// Sync behavior. Used by the background/auto-sync path.
	TriggerAuto TriggerPolicy = iota
	// TriggerManual loops syncBatchStep until the backlog is drained (or the
	// termination guard trips). Used by the mem_sync MCP tool.
	TriggerManual
)

// DrainState classifies how a Drain call ended. This is intentionally a
// minimal internal classification for PR 1b-ii — full DrainOutcome surfacing
// (mem_sync JSON field, health DTO wiring) is deferred to PR 3.
type DrainState int

const (
	// DrainFullySynced means the loop drained the backlog cleanly: the push
	// backlog was empty and the pull side reported no more pages.
	DrainFullySynced DrainState = iota
	// DrainExpectedPending means the loop stopped with backlog remaining
	// (TriggerAuto's single-step contract, or the no-progress guard tripping
	// without an error) — retrying later is expected to make progress.
	DrainExpectedPending
	// DrainDegradedFailure means a batch step returned an error and the loop
	// stopped because of it.
	DrainDegradedFailure
)

// DrainReason distinguishes WHY a Drain call ended in DrainExpectedPending —
// there are three different termination paths that all leave backlog
// remaining, and only some of them indicate something is actually stuck
// (PR 3, hive-sync-batched-drain, task 3.1):
//
//   - DrainReasonAutoSingleStep: TriggerAuto always stops after exactly one
//     batch by design (see the TriggerAuto branch in Drain). Backlog
//     remaining here is completely expected — the auto-sync tick will pick
//     up more on its next run. Not a signal of trouble.
//   - DrainReasonNoProgress: TriggerManual's no-progress guard tripped —
//     RecordsMarkedSynced was 0, the mutation cursor did not advance, AND
//     neither pull channel's cursor advanced for a full iteration. This means
//     a manual, "drain everything now" request could not make headway, which
//     usually means something IS stuck (permanent server-side rejection,
//     misbehaving server, or a persistent local write failure).
//   - DrainReasonIterationCap: TriggerManual hit maxDrainBatches — the loop
//     was still making progress on every iteration but ran out of allowed
//     batches. This is defense-in-depth against a pathologically large
//     backlog or an unanticipated gap in the no-progress guard; on a healthy
//     system it means "come back and drain again", not "stuck".
//
// DrainReason is empty ("") for DrainFullySynced and DrainDegradedFailure,
// where the distinction does not apply.
type DrainReason string

const (
	// DrainReasonNone is the zero value: no reason applies (DrainFullySynced,
	// DrainDegradedFailure, or TriggerAuto's DrainFullySynced-after-one-batch
	// case).
	DrainReasonNone DrainReason = ""
	// DrainReasonAutoSingleStep marks a TriggerAuto run that stopped after its
	// single allowed batch with backlog still remaining — expected by design.
	DrainReasonAutoSingleStep DrainReason = "auto-single-step"
	// DrainReasonNoProgress marks a TriggerManual run stopped by the
	// no-progress guard — nothing durably advanced for a full iteration.
	DrainReasonNoProgress DrainReason = "no-progress"
	// DrainReasonIterationCap marks a TriggerManual run stopped by the
	// maxDrainBatches defense-in-depth cap while still (apparently) making
	// progress.
	DrainReasonIterationCap DrainReason = "iteration-cap"
)

// DrainOutcome classifies how a Drain call ended (PR 3, hive-sync-batched-drain,
// task 3.1 — extends the PR 1b-ii minimal {State, Batches} shape with the full
// detail needed to surface drain health through mem_sync and the health DTO).
type DrainOutcome struct {
	// State is the coarse classification of the run (fully synced, expected
	// pending, or degraded failure).
	State DrainState
	// Reason distinguishes WHY State is DrainExpectedPending (see DrainReason
	// doc). Empty for DrainFullySynced and DrainDegradedFailure.
	Reason DrainReason
	// BatchesDone is the number of syncBatchStep iterations that actually ran
	// during this Drain call. Renamed from the PR 1b-ii `Batches` field name —
	// PR 3 adds BatchesRemaining alongside it, and the "Done" suffix makes the
	// pairing unambiguous at call sites and in the mem_sync/health JSON.
	BatchesDone int
	// BatchesRemaining is a best-effort estimate of how many more batches are
	// likely needed to fully drain the backlog. It is NOT precisely knowable
	// without another server round trip (the local backlog size does not
	// account for what the pull side still has queued), so this is left at
	// -1 ("unknown") whenever Drain cannot cheaply derive it. Today Drain does
	// not compute this value — it is populated only when a future caller can
	// supply it cheaply (e.g. from BacklogSize / syncPageSize on the final
	// batch). Documented rather than silently omitted so it always means the
	// same thing across the mem_sync and health surfaces.
	BatchesRemaining int
	// RemainingPush is a best-effort count of unsynced push work left after
	// this Drain run ended. It is populated from the LAST batch's
	// batchResult.BacklogSize (memories + sessions + prompts + pending
	// mutations fetched for that batch) — see the batchResult.BacklogSize doc
	// for why that number saturates at syncPageSize during a healthy
	// multi-page drain and is therefore only a floor, not an exact count, once
	// paging is involved. It is 0 when the run ended DrainFullySynced (the
	// push backlog was confirmed empty), and -1 when Drain could not run any
	// batch at all (e.g. this field is left unset before the loop runs).
	RemainingPush int
	// Err is set only when State is DrainDegradedFailure — the error the
	// failing batch returned. nil in every other case.
	Err error
}

// drainStateString maps a DrainState to the same wire vocabulary the
// mem_sync/health JSON surfaces use (mcp.drainStateJSON mirrors this — kept
// as two small unexported functions rather than a shared exported one to
// avoid adding a cross-package API surface just for a 3-way string switch).
// Used by RecordDrainOutcome persistence so the DB stores the same strings
// callers see over JSON.
func drainStateString(state DrainState) string {
	switch state {
	case DrainFullySynced:
		return "fully_synced"
	case DrainExpectedPending:
		return "expected_pending"
	case DrainDegradedFailure:
		return "degraded_failure"
	default:
		return "unknown"
	}
}

// Sync ejecuta un único ciclo de sync (push+pull) para un proyecto. Es un
// atajo sobre Drain con TriggerAuto — preserva la firma pública histórica
// (*Result, error) para no romper a mem_save autoSync ni a los callers
// existentes (design §4.3, PR 1b-ii).
func (s *Syncer) Sync(ctx context.Context, project string) (*Result, error) {
	result, _, err := s.Drain(ctx, project, TriggerAuto)
	return result, err
}

// Drain ejecuta uno o más ciclos de sync (push+pull) para un proyecto,
// según la TriggerPolicy (design §2.1, §4.3, PR 1b-ii):
//   - TriggerAuto corre exactamente un syncBatchStep (comportamiento
//     histórico de Sync).
//   - TriggerManual repite syncBatchStep hasta que el backlog de push esté
//     vacío y el pull no reporte más páginas, o hasta que el guard de
//     progreso (T1b-ii.5) detecte que una iteración no avanzó nada.
//
// La reserva de inFlight se toma UNA vez al principio y se mantiene durante
// todo el Drain — no se libera/reserva entre batches.
func (s *Syncer) Drain(ctx context.Context, project string, policy TriggerPolicy) (*Result, DrainOutcome, error) {
	if !s.tryReserve(project) {
		return nil, DrainOutcome{}, fmt.Errorf("%w: project %s", ErrSyncInFlight, project)
	}
	defer s.release(project)

	health, err := s.store.GetSyncHealth(project)
	if err != nil {
		return nil, DrainOutcome{}, fmt.Errorf("obtener estado de sync: %w", err)
	}

	now := s.deps.now().UTC()
	if !health.BackoffUntil.IsZero() && now.Before(health.BackoffUntil) {
		return nil, DrainOutcome{}, &BackoffError{Project: project, RetryAt: health.BackoffUntil}
	}

	if err := s.store.RecordSyncAttempt(project, now); err != nil {
		return nil, DrainOutcome{}, fmt.Errorf("registrar intento de sync: %w", err)
	}

	// Paso 1: JWT
	token, err := s.getOrRefreshToken(ctx)
	if err != nil {
		if recordErr := s.recordFailure(project, health, now, err); recordErr != nil {
			return nil, DrainOutcome{}, recordErr
		}
		return nil, DrainOutcome{}, fmt.Errorf("autenticación: %w", err)
	}
	s.retryPendingProjectBlockAcks(ctx, token)

	result := &Result{Project: project}
	outcome := DrainOutcome{}
	prevMutationCursor := db.MutationCursor{}
	// prevPullMemoriesCursor/prevPullSessionsCursor (PR 2b infinite-loop fix)
	// track each pull channel's cursor as of the END of the previous
	// iteration, so the no-progress guard below can require an actual
	// cursor advance instead of trusting the server's PullHasMore flag
	// unconditionally. Zero value matches "nothing persisted yet", exactly
	// like prevMutationCursor above.
	prevPullMemoriesCursor := db.PullCursor{}
	prevPullSessionsCursor := db.PullCursor{}

	// lastBatchBacklogSize tracks the most recent batch's batchResult.BacklogSize
	// so DrainOutcome.RemainingPush (PR 3, task 3.1) can report a best-effort
	// "work left" number without a second server round trip. See the
	// DrainOutcome.RemainingPush doc for why this is a floor, not an exact
	// count, once paging is involved.
	lastBatchBacklogSize := -1
	// A Drain owns at most one cached-token recovery regardless of its batch count.
	recoveryUsed := false

	for {
		if outcome.BatchesDone >= maxDrainBatches {
			// Defense-in-depth (PR 2b infinite-loop fix): even if the
			// cursor-advance corroboration below has a gap we haven't
			// anticipated, this hard cap guarantees Drain(TriggerManual)
			// cannot spin forever. Hitting this in practice means either a
			// pathological backlog far beyond the ~1.5k-5k target scale or a
			// genuine bug — surface it as pending (not a hard failure) so
			// the caller can retry, but make it loud in the log so it is
			// diagnosable.
			logger.Log.Printf("warn: Drain(project=%s) hit maxDrainBatches=%d without draining the backlog — stopping", project, maxDrainBatches)
			outcome.State = DrainExpectedPending
			outcome.Reason = DrainReasonIterationCap
			outcome.BatchesRemaining = -1
			outcome.RemainingPush = lastBatchBacklogSize
			break
		}
		batch, resp, stepErr := s.syncBatchStepWithResponse(ctx, project, token)
		if stepErr != nil && isMainSyncUnauthorized(stepErr) {
			if recoveryUsed {
				// A fresh token was already accepted earlier in this Drain. A later
				// 401 must never start another recovery loop.
				stepErr = fmt.Errorf("sync rejected reauthenticated session: %w", s.rejectReauthenticatedToken())
			} else {
				recoveryUsed = true
				recoveredToken, recoveryErr := s.recoverCachedToken(ctx, token)
				if recoveryErr != nil {
					stepErr = fmt.Errorf("recover cached Hive API session: %w", recoveryErr)
				} else {
					token = recoveredToken
					batch, resp, stepErr = s.syncBatchStepWithResponse(ctx, project, token)
					if stepErr != nil && isMainSyncUnauthorized(stepErr) {
						stepErr = fmt.Errorf("sync rejected reauthenticated session: %w", s.rejectReauthenticatedToken())
					}
				}
			}
		}
		if stepErr != nil {
			recordSyncErr := errors.Unwrap(stepErr)
			var blockedErr *ProjectBlockedError
			if errors.As(stepErr, &blockedErr) {
				if handleErr := s.handleProjectBlocked(ctx, token, project, blockedErr); handleErr != nil {
					stepErr = fmt.Errorf("%w: %v", stepErr, handleErr)
					recordSyncErr = stepErr
				}
			}
			// recordFailure must see the raw underlying error (unwrapped), not
			// the "<step label>: %w" wrapper syncBatchStepWithResponse adds —
			// that matches the pre-extraction behavior, where each inline step
			// passed its own raw err to recordFailure and only wrapped the
			// error it returned to its own caller. sanitizeRecordedSyncError
			// (health error text) depends on seeing the raw "sync failed (...)"
			// / "login failed (...)" prefix, so double-wrapping here would
			// corrupt the recorded health.LastError text.
			if recordErr := s.recordFailure(project, health, now, recordSyncErr); recordErr != nil {
				return nil, DrainOutcome{}, recordErr
			}
			outcome.State = DrainDegradedFailure
			outcome.Reason = DrainReasonNone
			outcome.BatchesDone++
			outcome.BatchesRemaining = -1
			outcome.RemainingPush = lastBatchBacklogSize
			outcome.Err = stepErr
			// Persist the degraded outcome (PR 3, task 3.4) before returning —
			// both the nil-result and partial-result branches below are
			// terminal for this Drain call, so this is the last chance to
			// record it. Best-effort: a persistence failure here must not mask
			// the original stepErr the caller is already about to see.
			if persistErr := s.store.RecordDrainOutcome(project, drainStateString(outcome.State), string(outcome.Reason), outcome.RemainingPush); persistErr != nil {
				logger.Log.Printf("warn: RecordDrainOutcome(project=%s): %v", project, persistErr)
			}
			if outcome.BatchesDone == 1 {
				// Preserve the historical single-batch Sync contract: a
				// first-and-only-batch failure returns a nil result.
				return nil, outcome, stepErr
			}
			return result, outcome, stepErr
		}

		outcome.BatchesDone++
		lastBatchBacklogSize = batch.BacklogSize
		result.Pushed += batch.Pushed
		result.Pulled += batch.Pulled
		result.Conflicts += resp.Conflicts
		result.PromptsPushed += resp.PromptsPushed
		result.MutationsPushed += batch.MutationsPushed
		result.MutationsPulled += batch.MutationsPulled
		result.CompatibilityMode = batch.CompatibilityMode
		result.MutationCursor = batch.MutationCursor

		if policy == TriggerAuto {
			if batch.PushBacklogEmpty && !batch.PullHasMore {
				outcome.State = DrainFullySynced
				outcome.RemainingPush = 0
			} else {
				outcome.State = DrainExpectedPending
				outcome.Reason = DrainReasonAutoSingleStep
				outcome.BatchesRemaining = -1
				outcome.RemainingPush = lastBatchBacklogSize
			}
			break
		}

		// TriggerManual: keep looping until there is nothing left to push and
		// the server reports no more pull pages.
		if batch.PushBacklogEmpty && !batch.PullHasMore {
			outcome.State = DrainFullySynced
			outcome.RemainingPush = 0
			break
		}

		// Termination guard (T1b-ii.5, revised design §4.3 for PR 1b-iii, and
		// again for PR 2b task 2.8's pull pagination, and again for the PR 2b
		// fresh-review infinite-loop fix): if this iteration durably marked
		// NOTHING synced, the mutation cursor did not advance, AND the pull
		// side did not actually advance either cursor, the loop is not
		// making progress (e.g. a permanent server-side conflict, or a
		// misbehaving server) — stop instead of spinning forever.
		//
		// pullAdvanced corroborates batch.PullHasMore with an actual cursor
		// move (PR 2b infinite-loop fix, fresh-context review CRITICAL #1):
		// PullHasMore alone is NOT trustworthy as a progress signal — hive-api
		// could report has_more=true while returning the same/nil next
		// cursor (server bug, replayed page, or a page whose rows are all
		// already local so nothing gets pulled/marked). Trusting PullHasMore
		// unconditionally in that case spins Drain(TriggerManual) forever,
		// since pull-apply does not increment RecordsMarkedSynced and the
		// mutation cursor is unrelated to the pull channels. Requiring an
		// actual per-channel cursor advance closes that hole while still
		// preserving the original intent: a HEALTHY multi-page pull-only
		// drain (cursor genuinely advances every page) must keep looping
		// until has_more=false — see
		// TestDrain_TriggerManual_DrainsMultiPagePullAndAdvancesCursorsPerChannel,
		// which stays green because postBatchPullMemoriesCursor/
		// postBatchPullSessionsCursor advance on every one of its 3 batches.
		pullAdvanced := batch.PullMemoriesCursor != prevPullMemoriesCursor || batch.PullSessionsCursor != prevPullSessionsCursor
		//
		// This intentionally no longer looks at BacklogSize. Once syncBatchStep
		// pages its push fetch at syncPageSize (PR 1b-iii), a healthy multi-page
		// drain can report the same (page-sized) BacklogSize on every iteration
		// even though the underlying backlog is shrinking — that would make the
		// old "BacklogSize >= prevBacklogSize" check false-positive and cut a
		// still-productive drain short. RecordsMarkedSynced reflects records the
		// server actually confirmed and the store durably marked this batch,
		// which stays a true progress signal regardless of page size.
		if batch.RecordsMarkedSynced == 0 && batch.MutationCursor == prevMutationCursor && !pullAdvanced {
			outcome.State = DrainExpectedPending
			outcome.Reason = DrainReasonNoProgress
			outcome.BatchesRemaining = -1
			outcome.RemainingPush = lastBatchBacklogSize
			break
		}
		prevMutationCursor = batch.MutationCursor
		prevPullMemoriesCursor = batch.PullMemoriesCursor
		prevPullSessionsCursor = batch.PullSessionsCursor
	}

	// Persist the drain outcome (PR 3, task 3.4) for the success path
	// (DrainFullySynced or DrainExpectedPending) — best-effort, mirroring the
	// degraded-failure path above: a persistence failure here must not fail
	// an otherwise-successful Drain call.
	if persistErr := s.store.RecordDrainOutcome(project, drainStateString(outcome.State), string(outcome.Reason), outcome.RemainingPush); persistErr != nil {
		logger.Log.Printf("warn: RecordDrainOutcome(project=%s): %v", project, persistErr)
	}

	// Paso 6: actualizamos el timestamp del último sync exitoso
	if err := s.store.RecordSyncSuccess(project, now); err != nil {
		return nil, DrainOutcome{}, fmt.Errorf("registrar éxito de sync: %w", err)
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
		SyncCountsJSON: syncCountsJSON(result.Pushed, result.Pulled, result.Conflicts, result.PromptsPushed, result.MutationsPushed, result.MutationsPulled),
		MetadataJSON:   successAttemptMetadata(recoveryUsed),
	}
	if err := s.store.RecordSyncAttemptLog(ctx, attemptLog); err != nil {
		logger.Log.Printf("warn: record sync attempt log %s: %v", attemptLog.AttemptID, err)
	} else {
		s.deleteExpiredSyncAttemptLogs(ctx)
		s.flushSyncAttemptLogs(ctx, token)
	}

	return result, outcome, nil
}

func (s *Syncer) handleProjectBlocked(ctx context.Context, token, localProject string, blockedErr *ProjectBlockedError) error {
	if blockedErr == nil {
		return nil
	}
	cmd := db.ProjectBlockCommand{
		CommandID:           blockedErr.Command.CommandID,
		AckToken:            blockedErr.Command.AckToken,
		Project:             blockedErr.Command.Project,
		CanonicalProjectKey: blockedErr.Command.CanonicalProjectKey,
		Reason:              blockedErr.Command.Reason,
		BlockedAt:           blockedErr.Command.BlockedAt,
	}
	block, err := s.store.RecordProjectBlock(ctx, cmd)
	if err != nil {
		return fmt.Errorf("record local project block: %w", err)
	}
	ack := db.ProjectBlockAck{
		CommandID:           block.CommandID,
		AckToken:            block.AckToken,
		CanonicalProjectKey: block.CanonicalProjectKey,
		Status:              db.ProjectBlockAckApplied,
		AppliedAt:           s.deps.now().UTC(),
	}
	quarantine, err := s.store.QuarantineBlockedProject(ctx, localProject, "hive-daemon", "project blocked by Hive API", ack.AppliedAt)
	if err != nil {
		ack.Status = db.ProjectBlockAckFailed
		ack.Warning = err.Error()
	} else if quarantine.Warning != "" {
		ack.Warning = quarantine.Warning
	}
	if err := s.store.RecordPendingProjectBlockAck(ctx, ack); err != nil {
		return fmt.Errorf("record pending project block ack: %w", err)
	}
	if err := s.client.ackProjectBlock(ctx, token, ack); err != nil {
		return fmt.Errorf("report project block ack: %w", err)
	}
	if _, err := s.store.RecordProjectBlockAck(ctx, ack); err != nil {
		return fmt.Errorf("record local project block ack: %w", err)
	}
	return nil
}

const pendingProjectBlockAckRetryLimit = 25

func (s *Syncer) retryPendingProjectBlockAcks(ctx context.Context, token string) {
	pending, err := s.store.ListPendingProjectBlockAcks(ctx, pendingProjectBlockAckRetryLimit)
	if err != nil {
		logger.Log.Printf("warn: list pending project block ACKs: %v", err)
		return
	}
	for _, ack := range pending {
		if ack.AppliedAt.IsZero() {
			ack.AppliedAt = s.deps.now().UTC()
		}
		if err := s.client.ackProjectBlock(ctx, token, ack); err != nil {
			logger.Log.Printf("warn: retry project block ACK command_id=%s canonical_project_key=%s: %v", ack.CommandID, ack.CanonicalProjectKey, err)
			continue
		}
		if _, err := s.store.RecordProjectBlockAck(ctx, ack); err != nil {
			logger.Log.Printf("warn: record retried project block ACK command_id=%s canonical_project_key=%s: %v", ack.CommandID, ack.CanonicalProjectKey, err)
		}
	}
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
	// Paso 2b: sesiones locales pendientes de sync, paged (non-fatal si falla).
	// Fetched BEFORE memories so the session-priority gate below (PR 2b,
	// hive-sync-batched-drain) can decide whether this batch is allowed to
	// push memories at all.
	unsyncedSessions, err := s.store.ListUnsyncedSessionsPage(project, syncPageSize)
	if err != nil {
		logger.Log.Printf("warn: obtener sesiones no sincronizadas: %v", err)
		unsyncedSessions = nil
	}

	// Paso 2: memorias locales pendientes de sync. Paged at syncPageSize (PR
	// 1b-iii, hive-sync-batched-drain) so a single push batch never exceeds
	// the page cap — a Drain(TriggerManual) run pages through the rest via
	// repeated syncBatchStep calls instead of pushing the whole backlog at
	// once.
	//
	// Session-priority gate (PR 2b, hive-sync-batched-drain — fixes an
	// FK-ordering regression from PR 1b-iii): PR 1b-iii paged the sessions
	// and memories channels INDEPENDENTLY. hive-api validates the
	// memories[].session_id FK per-request against that SAME request's
	// sessions[] plus whatever sessions the server already has confirmed. If
	// the session backlog spans more than one page, a memory in an early
	// memories page can name a session sitting in a LATER, not-yet-pushed
	// session page — the server would then 400 with ErrSessionNotFound.
	//
	// The fix: while any unsynced session remains for this project, this
	// batch pushes sessions (+prompts+mutations) only, and defers ALL
	// memories to a later batch/tick — it does not even fetch a memories
	// page. Once ListUnsyncedSessionsPage reports the session channel empty,
	// memories resume flowing normally. This composes with the no-progress
	// guard because draining sessions IS progress (RecordsMarkedSynced
	// increments), and it holds for both TriggerManual (the loop keeps
	// calling syncBatchStep) and TriggerAuto (that single tick simply pushes
	// sessions only; memories wait for the next auto tick once sessions are
	// fully drained).
	//
	// Deferring the fetch itself (rather than fetching and holding back) also
	// keeps len(unsyncedSessions) > 0 driving PushBacklogEmpty below to
	// false, so Drain(TriggerManual) correctly keeps looping instead of
	// mistaking a memories-deferred batch for a fully-drained one.
	var unsynced []*models.Memory
	var pendingMutations []db.MutationEnvelope
	if len(unsyncedSessions) == 0 {
		unsynced, err = s.store.GetUnsyncedPage(project, syncPageSize)
		if err != nil {
			return batchResult{}, nil, fmt.Errorf("obtener memorias no sincronizadas: %w", err)
		}

		// Paso 2d: mutaciones locales pendientes para protocolo v2. Already
		// capped at syncPageSize — GetPendingMutations has taken an explicit
		// limit since PR 1a. Gated by the SAME session-priority check as
		// memories above (fix-mutation-sync-session-gate): a mutation's
		// Memory payload can carry a memories.session_id FK
		// (mutation.Memory.SessionID) exactly like a legacy memory row, so it
		// is exposed to the identical FK-ordering hazard the memories gate
		// above was built to fix — a mutation naming a session sitting in a
		// LATER, not-yet-pushed session page must not be pushed before that
		// session is confirmed. Fetching this only once the session channel
		// is empty (rather than fetching and holding back) mirrors the
		// memories gate exactly.
		pendingMutations, err = s.store.GetPendingMutations(project, syncPageSize)
		if err != nil {
			return batchResult{}, nil, fmt.Errorf("obtener mutaciones pendientes: %w", err)
		}
	} else {
		// Visibility improvement (PR 2b fresh-review WARNING #2): log once per
		// gated batch so a session that never drains (and therefore keeps
		// deferring this project's memories and mutations indefinitely) is
		// diagnosable from the daemon log instead of silently starving them
		// forever. Low noise by design: one informative line per gated step,
		// not per session. The infinite-loop risk this used to carry (a
		// permanently stuck session with pull pages still pending) is now
		// bounded by the Drain no-progress guard's pull-cursor-advance
		// corroboration and the maxDrainBatches cap above, not by this log
		// line.
		logger.Log.Printf("info: sync project=%s deferring memories and mutations this batch — %d unsynced session(s) still pending", project, len(unsyncedSessions))
	}

	// Paso 2c: prompts locales pendientes de sync, paged (non-fatal si falla).
	unsyncedPrompts, err := s.store.GetUnsyncedPromptsPage(ctx, project, syncPageSize)
	if err != nil {
		logger.Log.Printf("warn: obtener prompts no sincronizados: %v", err)
		unsyncedPrompts = nil
	}

	mutationCursor, err := s.store.GetMutationCursor(mutationCursorConsumerAPI, project)
	if err != nil {
		return batchResult{}, nil, fmt.Errorf("obtener cursor de mutaciones: %w", err)
	}

	// Paso 2e: cursores de pull acotado persistidos (PR 2a/2b,
	// hive-sync-batched-drain task 2.8). Cada canal pagina de forma
	// independiente — ver GetPullCursor/SetPullCursor doc. Un cursor ausente
	// (primera vez, o canal ya completamente drenado) es su valor cero, que
	// pullOptions envía como nil (omitempty), exactamente como "empezar desde
	// el principio de la ventana since-based actual".
	pullMemoriesCursor, err := s.store.GetPullCursor(mutationCursorConsumerAPI, project, pullCursorChannelMemories)
	if err != nil {
		return batchResult{}, nil, fmt.Errorf("obtener cursor de pull de memorias: %w", err)
	}
	pullSessionsCursor, err := s.store.GetPullCursor(mutationCursorConsumerAPI, project, pullCursorChannelSessions)
	if err != nil {
		return batchResult{}, nil, fmt.Errorf("obtener cursor de pull de sesiones: %w", err)
	}

	// PushBacklogEmpty: nothing fetched above means nothing remained to send
	// for this step. See the batchResult field doc for why this is the
	// cheapest accurate check for a single, non-paged batch step.
	backlogSize := len(unsynced) + len(unsyncedSessions) + len(unsyncedPrompts) + len(pendingMutations)
	pushBacklogEmpty := backlogSize == 0

	now := s.deps.now().UTC()

	// Paso 3 + 4: sync bidireccional con el servidor. PullLimit=syncPageSize
	// is the explicit opt-in into hive-api's bounded pull pagination (PR 2a)
	// — omitting it would fall back to an unbounded legacy pull.
	lastSync, _ := s.store.GetLastSync(project)
	var lastSyncPtr *time.Time
	if !lastSync.IsZero() {
		lastSyncPtr = &lastSync
	}

	resp, err := s.client.sync(ctx, token, project, unsyncedSessions, unsynced, unsyncedPrompts, lastSyncPtr, pendingMutations, &mutationCursor, pullOptions{
		Limit:          syncPageSize,
		MemoriesCursor: pullCursorOrNil(pullMemoriesCursor),
		SessionsCursor: pullCursorOrNil(pullSessionsCursor),
	})
	if err != nil {
		return batchResult{}, nil, fmt.Errorf("sync con servidor: %w", err)
	}

	compatibilityMode := compatibilityModeLegacy
	if resp.CompatibilityMode != "" {
		compatibilityMode = resp.CompatibilityMode
	}

	// recordsMarkedSynced counts records durably marked synced_at THIS batch —
	// the PR 1b-iii progress signal for the Drain no-progress guard (see the
	// batchResult.RecordsMarkedSynced doc). Only successful marks count: a
	// MarkXSynced error means the store did not durably confirm that row, so
	// it must not be treated as progress.
	recordsMarkedSynced := 0

	// Paso 5a: marcamos como sincronizadas las sesiones que enviamos
	for _, sess := range unsyncedSessions {
		if err := s.store.MarkSessionSynced(sess.ID, now); err != nil {
			logger.Log.Printf("warn: MarkSessionSynced %s: %v", sess.ID, err)
			continue
		}
		recordsMarkedSynced++
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
				continue
			}
			recordsMarkedSynced++
		}
	}

	// Paso 5c: marcamos como sincronizados los prompts que enviamos (non-fatal)
	for _, p := range unsyncedPrompts {
		if err := s.store.MarkPromptSynced(ctx, p.SyncID, now); err != nil {
			logger.Log.Printf("warn: MarkPromptSynced %s: %v", p.SyncID, err)
			continue
		}
		recordsMarkedSynced++
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
		recordsMarkedSynced += mutationsPushed
	}

	// Paso 5f: persistimos o limpiamos los cursores de pull acotado para el
	// próximo batch. A channel reporting has_more=false is fully drained, so its
	// durable cursor is cleared and the post-batch cursor becomes zero. A channel
	// reporting has_more=true only advances when the server sends a next cursor;
	// has_more=true with a nil next cursor leaves the previous cursor intact and
	// lets Drain's no-progress guard handle the stalled pagination state.
	postBatchPullMemoriesCursor := pullMemoriesCursor
	postBatchPullSessionsCursor := pullSessionsCursor
	if !resp.PulledHasMore {
		if err := s.store.ClearPullCursor(mutationCursorConsumerAPI, project, pullCursorChannelMemories); err != nil {
			return batchResult{}, nil, fmt.Errorf("clear pull cursor de memorias: %w", err)
		}
		postBatchPullMemoriesCursor = db.PullCursor{}
	} else if resp.NextPullCursor != nil {
		if err := s.store.SetPullCursor(mutationCursorConsumerAPI, project, pullCursorChannelMemories, *resp.NextPullCursor, now); err != nil {
			return batchResult{}, nil, fmt.Errorf("guardar cursor de pull de memorias: %w", err)
		}
		postBatchPullMemoriesCursor = *resp.NextPullCursor
	}
	if !resp.PulledSessionsHasMore {
		if err := s.store.ClearPullCursor(mutationCursorConsumerAPI, project, pullCursorChannelSessions); err != nil {
			return batchResult{}, nil, fmt.Errorf("clear pull cursor de sesiones: %w", err)
		}
		postBatchPullSessionsCursor = db.PullCursor{}
	} else if resp.NextSessionCursor != nil {
		if err := s.store.SetPullCursor(mutationCursorConsumerAPI, project, pullCursorChannelSessions, *resp.NextSessionCursor, now); err != nil {
			return batchResult{}, nil, fmt.Errorf("guardar cursor de pull de sesiones: %w", err)
		}
		postBatchPullSessionsCursor = *resp.NextSessionCursor
	}

	return batchResult{
		Pushed:              resp.Pushed,
		Pulled:              len(resp.Pulled),
		MutationsPushed:     mutationsPushed,
		MutationsPulled:     mutationsPulled,
		CompatibilityMode:   compatibilityMode,
		MutationCursor:      mutationCursor,
		PushBacklogEmpty:    pushBacklogEmpty,
		RecordsMarkedSynced: recordsMarkedSynced,
		BacklogSize:         backlogSize,
		// PullHasMore (PR 2a/2b): true when EITHER pull channel reports more
		// pages pending. An old hive-api response with no has_more fields at
		// all decodes both to false (see syncResponse doc), so Drain
		// terminates on push-empty exactly like before PR 2b — no behavior
		// change against a server that predates pagination.
		PullHasMore:        resp.PulledHasMore || resp.PulledSessionsHasMore,
		PullMemoriesCursor: postBatchPullMemoriesCursor,
		PullSessionsCursor: postBatchPullSessionsCursor,
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

// syncCountsJSON builds the audit payload persisted on SyncAttemptLog. All
// six counts MUST come from the same accumulation scope (see
// hive-sync-batched-drain Judgment Day A1): for a Drain(TriggerManual) run
// that loops over several batches, that means the totals accumulated across
// every batch, not a single batch's raw syncResponse. Taking pushed/pulled/
// conflicts/prompts_pushed from only the last batch while mutations_pushed/
// mutations_pulled were already accumulated produced an internally
// inconsistent audit record for multi-batch drains. Passing explicit ints
// (rather than a *syncResponse) makes that invariant obvious at every call
// site and keeps the single-batch case correct for free: with exactly one
// batch, the accumulated totals equal that batch's own counts.
func syncCountsJSON(pushed, pulled, conflicts, promptsPushed, mutationsPushed, mutationsPulled int) string {
	counts := map[string]int{
		"pushed":           pushed,
		"pulled":           pulled,
		"conflicts":        conflicts,
		"prompts_pushed":   promptsPushed,
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

// pullCursorOrNil converts a persisted db.PullCursor into the pointer shape
// pullOptions expects: a zero-value cursor (never persisted, or the channel
// has no resume position yet) sends nil so the request's pull_cursor/
// pull_session_cursor field is omitted entirely (omitempty) — "start from
// the beginning of the current window" — rather than an explicit zero
// timestamp, which hive-api would otherwise have to special-case.
func pullCursorOrNil(cursor db.PullCursor) *PullCursor {
	if cursor == (db.PullCursor{}) {
		return nil
	}
	result := cursor
	return &result
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

func (s *Syncer) tryReserve(project string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inFlight[project] {
		return false
	}
	s.inFlight[project] = true
	return true
}

func (s *Syncer) release(project string) {
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
	httpStatus := syncAttemptHTTPStatus(message)
	errorCode := "sync_failed"
	metadata := "{}"
	var statusErr *HTTPStatusError
	if errors.As(syncErr, &statusErr) {
		httpStatus = statusErr.StatusCode
	}
	if errors.Is(syncErr, ErrStoredCredentialsRejected) {
		httpStatus = 401
		errorCode = "auth_credentials_stale"
		metadata = `{"auth_recovery":"stopped"}`
	}
	if errors.Is(syncErr, ErrReauthenticatedTokenRejected) {
		httpStatus = 401
		errorCode = "auth_token_rejected_after_login"
	}
	if errors.Is(syncErr, ErrAuthReloginFailed) {
		errorCode = "auth_relogin_failed"
	}
	attemptLog := db.SyncAttemptLog{
		AttemptID:      s.deps.newAttemptID(),
		DevID:          s.client.cfg.Email,
		Project:        project,
		Client:         "hive-daemon",
		StartedAt:      startedAt,
		EndedAt:        s.deps.now().UTC(),
		Outcome:        db.SyncAttemptOutcomeFailure,
		HTTPStatus:     httpStatus,
		ErrorCode:      errorCode,
		ErrorMessage:   message,
		SyncCountsJSON: "{}",
		MetadataJSON:   metadata,
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

// getOrRefreshToken returns a cached JWT or makes one initial login attempt.
// A rejected stored credential is process-latched until the daemon restarts.
func (s *Syncer) getOrRefreshToken(ctx context.Context) (string, error) {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	if err := s.latchedAuthErrorLocked(); err != nil {
		return "", err
	}
	if token := s.store.GetJWT(); token != "" {
		return token, nil
	}
	return s.loginLocked(ctx, false)
}

// recoverCachedToken is the sole owner of cached-token recovery for a Drain.
// The caller has already observed a main-sync 401 and guarantees one call.
func (s *Syncer) recoverCachedToken(ctx context.Context, rejectedToken string) (string, error) {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	if err := s.latchedAuthErrorLocked(); err != nil {
		return "", err
	}
	if token := s.store.GetJWT(); token != "" && token != rejectedToken {
		return token, nil
	}
	if err := s.store.ClearJWT(); err != nil {
		return "", ErrCachedSessionCleanupFailed
	}
	return s.loginLocked(ctx, true)
}

func (s *Syncer) loginLocked(ctx context.Context, recovery bool) (string, error) {
	token, expiresAt, err := s.client.login(ctx)
	if err != nil {
		var statusErr *HTTPStatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == 401 {
			if err := s.store.ClearJWT(); err != nil {
				return "", ErrCachedSessionCleanupFailed
			}
			s.credentialsStale = true
			if !s.staleDiagnosticEmitted {
				s.staleDiagnosticEmitted = true
				logger.Log.Printf("warn: %s", ErrStoredCredentialsRejected)
			}
			return "", ErrStoredCredentialsRejected
		}
		if recovery {
			return "", fmt.Errorf("%w: %w", ErrAuthReloginFailed, err)
		}
		return "", err
	}
	if err := s.store.SetJWT(token, expiresAt); err != nil {
		if recovery {
			// Storage errors may contain local implementation details. Recovery
			// remains transient and must not latch or report a successful session.
			return "", ErrAuthReloginFailed
		}
		return "", fmt.Errorf("store login session: %w", err)
	}
	return token, nil
}

func (s *Syncer) rejectReauthenticatedToken() error {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	if err := s.store.ClearJWT(); err != nil {
		return ErrCachedSessionCleanupFailed
	}
	s.reauthenticatedTokenRejected = true
	if !s.tokenRejectedDiagnosticEmitted {
		s.tokenRejectedDiagnosticEmitted = true
		logger.Log.Printf("warn: %s", ErrReauthenticatedTokenRejected)
	}
	return ErrReauthenticatedTokenRejected
}

func isMainSyncUnauthorized(err error) bool {
	var statusErr *HTTPStatusError
	return errors.As(err, &statusErr) && statusErr.Operation == "sync" && statusErr.StatusCode == 401
}

func successAttemptMetadata(recoveryUsed bool) string {
	if recoveryUsed {
		return `{"auth_recovery":"token_refreshed"}`
	}
	return "{}"
}

func (s *Syncer) latchedAuthErrorLocked() error {
	if s.credentialsStale {
		return ErrStoredCredentialsRejected
	}
	if s.reauthenticatedTokenRejected {
		return ErrReauthenticatedTokenRejected
	}
	return nil
}
