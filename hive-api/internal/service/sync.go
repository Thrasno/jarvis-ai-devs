package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
)

// Sentinel errors surfaced by Push for handler-level 4xx classification (R2-CRIT-6).
// Handlers must use errors.Is to detect them — wrapped fmt.Errorf chains preserve identity.
var (
	// ErrSessionProjectMismatch — incoming session_id (e.g. manual-save-other) does
	// not belong to the request's project. Handler should return 400 Bad Request.
	ErrSessionProjectMismatch = errors.New("session project mismatch with request project")

	// ErrSessionNotFound — incoming session_id is unknown server-side and not present
	// in the push payload's sessions[]. Handler should return 400 Bad Request.
	ErrSessionNotFound = errors.New("session not found on server and not in push payload")

	// ErrPromptProjectMismatch — a prompt payload names a project other than the
	// one the request claims. The prompt counterpart of ErrSessionProjectMismatch;
	// handler should return 400 Bad Request.
	ErrPromptProjectMismatch = errors.New("prompt project mismatch with request project")
)

const syncMutationPullBatchSize = 100

// SyncService gestiona la sincronización bidireccional entre el daemon local
// y el servidor central.
//
// Protocolo de sync:
//  1. El daemon envía sus memorias locales (Push).
//     El servidor decide cuáles acepta según el algoritmo de 4 ramas.
//  2. El servidor envía las memorias que el daemon no tiene (Pull).
//     El daemon las guarda en su SQLite local.
//
// Push y Pull son independientes: el daemon puede hacer solo Push, solo Pull,
// o ambos en secuencia. El orden recomendado es Push primero, luego Pull,
// para que el Pull excluya las memorias que acaban de ser enviadas.
type SyncService interface {
	// Push recibe un batch de memorias del daemon y las persiste en el servidor.
	// Devuelve estadísticas: cuántas se guardaron y cuántas generaron conflicto.
	//
	// La lógica de resolución de conflictos está en MemoryRepository.Upsert.
	// SyncService interpreta el resultado para contar pushed vs conflicts.
	Push(ctx context.Context, req model.SyncRequest, userID string) (*model.SyncResponse, error)
	Sync(ctx context.Context, req model.SyncRequest, userID string) (*model.SyncResponse, error)

	// PullAll devuelve sesiones Y memorias actualizadas después de 'since'.
	// Sessions se devuelven PRIMERO para que el daemon receptor satisfaga la FK.
	//
	// limit acota cuántas filas se devuelven POR CANAL (memorias y sesiones se
	// paginan independientemente, cada una con su propio cursor) — ya debe venir
	// normalizado por el caller (el handler aplica model.ClampPullLimit antes de
	// llegar aquí). limit <= 0 (model.UnboundedPullLimit) significa "sin límite" —
	// el repositorio hace un barrido completo sin LIMIT y siempre reporta
	// hasMore=false; esto preserva el comportamiento pre-2a para clientes que no
	// enviaron pull_limit. memoriesCursor/sessionsCursor reanudan cada canal desde
	// una página anterior; su valor cero (model.PullCursor{}) significa "desde el
	// principio del barrido since".
	//
	// PullAll NO compone un drain completo — devuelve exactamente una página por
	// canal y dice si hay más (PullResult.MemoriesHasMore / SessionsHasMore). La
	// composición de múltiples páginas hasta agotar el backlog es responsabilidad
	// del daemon consumidor (PR 2b), no de este servicio.
	PullAll(ctx context.Context, project string, since time.Time, excludeSyncIDs []string, limit int, memoriesCursor, sessionsCursor model.PullCursor) (*model.PullResult, error)
}

type syncService struct {
	repo        repository.MemoryRepository
	promptRepo  repository.PromptRepository
	sessionRepo repository.SessionRepository
	auditRepo   repository.AuditRepository
	blockRepo   repository.ProjectBlockRepository
	tx          repository.TxManager
}

// NewSyncService crea el SyncService con los repositorios inyectados.
// memRepo gestiona memorias; promptRepo gestiona user-prompts;
// sessionRepo gestiona sesiones (requerido para ordering FK en push).
func NewSyncService(memRepo repository.MemoryRepository, promptRepo repository.PromptRepository, sessionRepo repository.SessionRepository, auditRepo repository.AuditRepository, blockRepo repository.ProjectBlockRepository, tx ...repository.TxManager) SyncService {
	var txManager repository.TxManager
	if len(tx) > 0 {
		txManager = tx[0]
	}
	return &syncService{repo: memRepo, promptRepo: promptRepo, sessionRepo: sessionRepo, auditRepo: auditRepo, blockRepo: blockRepo, tx: txManager}
}

// Push procesa el batch de memorias del cliente.
//
// Orden: (1) sessions, (2) memories, (3) prompts.
// Sessions se procesan PRIMERO para satisfacer la FK memories.session_id → sessions(id).
//
// Para memories:
//
//	Upsert devuelve (mem, true,  nil) → fue INSERT       → pushed++
//	Upsert devuelve (mem, false, nil) → fue UPDATE       → pushed++
//	Upsert devuelve (nil, false, nil) → fue SKIP         → conflicts++
//	Upsert devuelve (_,   _,    err)  → error de BD      → propagamos error
//
// El campo CreatedBy se asigna aquí, en el service — el repositorio no sabe
// quién está haciendo el sync. Ese dato viene del JWT (validado por el middleware).
func (s *syncService) Push(ctx context.Context, req model.SyncRequest, userID string) (*model.SyncResponse, error) {
	if err := model.ValidateSyncProjectIdentity(req); err != nil {
		return nil, err
	}
	return s.pushWithRepos(ctx, req, userID, syncPushRepos{Memory: s.repo, Prompt: s.promptRepo, Session: s.sessionRepo, Audit: s.auditRepo, ProjectBlocks: s.blockRepo})
}

func (s *syncService) Sync(ctx context.Context, req model.SyncRequest, userID string) (*model.SyncResponse, error) {
	if s.tx == nil {
		return nil, ErrProjectBlockUnavailable
	}
	if err := model.ValidateSyncProjectIdentity(req); err != nil {
		return nil, err
	}

	var resp *model.SyncResponse
	err := s.tx.WithinTx(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		txRepos := syncTransactionReposFrom(repos)
		if !txRepos.Valid() {
			return ErrProjectBlockUnavailable
		}
		keys := repository.ProjectLockKeys(syncRequestProjects(req))
		if err := txRepos.ProjectKeyLocks.LockProjectKeys(ctx, keys); err != nil {
			if errors.Is(err, repository.ErrProjectKeyLockBusy) {
				log.Printf("warn: project-key lock contention during sync projects=%q", keys)
			}
			return err
		}
		pushResp, err := s.pushWithRepos(ctx, req, userID, txRepos.Push)
		if err != nil {
			return err
		}
		resp, err = s.syncResponseWithPull(ctx, req, pushResp, txRepos.Pull)
		return err
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

type syncPushRepos struct {
	Memory        repository.MemoryRepository
	Prompt        repository.PromptRepository
	Session       repository.SessionRepository
	Audit         repository.AuditRepository
	ProjectBlocks repository.ProjectBlockRepository
}

type syncPullRepos struct {
	Memory  repository.MemoryRepository
	Session repository.SessionRepository
}

type syncTransactionRepos struct {
	Push            syncPushRepos
	Pull            syncPullRepos
	ProjectKeyLocks repository.ProjectKeyLockRepository
}

func syncTransactionReposFrom(repos repository.TxRepositories) syncTransactionRepos {
	return syncTransactionRepos{
		Push: syncPushRepos{
			Memory:        repos.Memory,
			Prompt:        repos.Prompt,
			Session:       repos.Session,
			Audit:         repos.Audit,
			ProjectBlocks: repos.ProjectBlocks,
		},
		Pull: syncPullRepos{
			Memory:  repos.Memory,
			Session: repos.Session,
		},
		ProjectKeyLocks: repos.ProjectKeyLocks,
	}
}

func (r syncTransactionRepos) Valid() bool {
	return r.Push.Memory != nil && r.Push.Prompt != nil && r.Push.Session != nil && r.Push.Audit != nil && r.Push.ProjectBlocks != nil && r.Pull.Memory != nil && r.Pull.Session != nil && r.ProjectKeyLocks != nil
}

func (s *syncService) pushWithRepos(ctx context.Context, req model.SyncRequest, userID string, repos syncPushRepos) (*model.SyncResponse, error) {
	if err := s.precheckBlockedProjects(ctx, req, repos.ProjectBlocks); err != nil {
		return nil, err
	}
	// ─── Fase 1: sessions (MUST run before memories) ─────────────────────────
	// Build a set of session IDs arriving in this payload so we can detect
	// non-sentinel unknown IDs in the memory loop below (T4.5).
	inPayloadSessions := make(map[string]bool, len(req.Sessions))
	for _, sp := range req.Sessions {
		// R4-FIX-2 — reject cross-project sessions BEFORE upsert so a malicious
		// daemon can't attribute a session to a project that differs from
		// req.Project and then have inPayload[] short-circuit the per-memory
		// validateSessionAttribution check.
		if sp.Project != req.Project {
			return nil, fmt.Errorf("session %q project mismatch: payload says %q, request says %q: %w",
				sp.ID, sp.Project, req.Project, ErrSessionProjectMismatch)
		}
		sess := &model.Session{
			ID:          sp.ID,
			SyncID:      sp.SyncID,
			Project:     sp.Project,
			FromProject: sp.FromProject,
			Directory:   sp.Directory,
			DevID:       sp.DevID,
			Client:      sp.Client,
			StartedAt:   sp.StartedAt,
			EndedAt:     sp.EndedAt,
			Summary:     sp.Summary,
		}
		if err := repos.Session.UpsertSession(ctx, sess); err != nil {
			return nil, fmt.Errorf("upsert session %s: %w", sp.ID, err)
		}
		inPayloadSessions[sp.ID] = true
	}

	// ─── Fase 2: memories ─────────────────────────────────────────────────────
	var pushed, conflicts int
	mutationProtocolCapable := req.ProtocolVersion >= model.MutationProtocolVersion
	mutationProtocolAuthoritative := mutationProtocolCapable && len(req.Mutations) > 0

	for _, payload := range req.Memories {
		if mutationProtocolAuthoritative {
			continue
		}

		// Resolve session_id for this memory (T4.4 + T4.5).
		sessionID, err := s.resolveSessionID(ctx, repos.Session, payload.SessionID, req.Project, inPayloadSessions)
		if err != nil {
			return nil, err
		}

		// Construimos el model.Memory a partir del payload del cliente.
		// El payload tiene los campos que envía el daemon; lo adaptamos
		// al modelo interno del servidor.
		mem := &model.Memory{
			SyncID:        payload.SyncID,
			Project:       payload.Project,
			TopicKey:      payload.TopicKey,
			Category:      payload.Category,
			Title:         payload.Title,
			Content:       payload.Content,
			Tags:          payload.Tags,
			FilesAffected: payload.FilesAffected,
			CreatedBy:     userID, // sobreescribimos con el ID del JWT, no el del payload
			CreatedAt:     payload.CreatedAt,
			UpdatedAt:     payload.UpdatedAt,
			SessionID:     sessionID,
		}

		saved, wasInsert, err := repos.Memory.Upsert(ctx, mem)
		if err != nil {
			if errors.Is(err, repository.ErrProjectBlocked) {
				return nil, projectBlockedErrorForProjectDelivery(ctx, repos.ProjectBlocks, mem.Project, req.AckSubject)
			}
			return nil, err
		}

		if saved == nil {
			// nil → el servidor rechazó la memoria (Ramas 2 y 3).
			// El daemon sabe que su versión fue ignorada.
			conflicts++
		} else {
			// Non-nil → la memoria fue guardada (Ramas 1 y 4).
			// wasInsert distingue INSERT de UPDATE, pero ambos cuentan como pushed.
			_ = wasInsert
			pushed++
		}
	}

	// --- Fase de prompts ---
	// Iteramos los user-prompts del request y los persistimos de forma idempotente.
	// El repositorio usa ON CONFLICT DO NOTHING — si el sync_id ya existe, no hace nada.
	// PromptsPushed cuenta solo los Upsert que resultaron en INSERT (saved=true).
	// Re-sync de prompts ya conocidos no incrementa el contador.
	var promptsPushed int
	for _, payload := range req.Prompts {
		// The prompt counterpart of the session check above. Quarantine and the
		// project-key lock already cover both ends of a prompt relocation
		// (syncRequestProjects collects Project and FromProject), so this is not
		// an authorization gain — it is an attribution one. emitSyncAudit books
		// the whole sync under req.Project, so a prompt written into, or
		// relocated into, some other project was recorded against a project that
		// never touched it. The audit trail is what an operator reads to answer
		// "who moved this".
		if payload.Project != req.Project {
			return nil, fmt.Errorf("prompt %q project mismatch: payload says %q, request says %q: %w",
				payload.SyncID, payload.Project, req.Project, ErrPromptProjectMismatch)
		}
		p := &model.Prompt{
			SyncID:      payload.SyncID,
			Project:     payload.Project,
			FromProject: payload.FromProject,
			Content:     payload.Content,
			CreatedBy:   userID,
			CreatedAt:   payload.CreatedAt,
		}
		saved, err := repos.Prompt.Upsert(ctx, p)
		if err != nil {
			if errors.Is(err, repository.ErrProjectBlocked) {
				return nil, projectBlockedErrorForProjectDelivery(ctx, repos.ProjectBlocks, p.Project, req.AckSubject)
			}
			return nil, err
		}
		if saved {
			promptsPushed++
		}
	}

	var pulledMutations []model.MutationEnvelope
	var mutationResults []model.MutationApplyResult
	var nextMutationCursor *model.MutationCursor
	compatibilityMode := ""
	if mutationProtocolAuthoritative {
		compatibilityMode = model.CompatibilityModeMutationV2
		for _, mutation := range req.Mutations {
			if mutationProjectMismatch(mutation, req.Project) {
				conflicts++
				mutationResults = append(mutationResults, rejectedMutation(mutation, req.Project,
					"mutation project does not match sync project"))
				continue
			}

			result, err := repos.Memory.ApplyMemoryMutation(ctx, mutation)
			if err != nil {
				if errors.Is(err, repository.ErrProjectBlocked) {
					return nil, projectBlockedErrorForProjectDelivery(ctx, repos.ProjectBlocks, mutation.Project, req.AckSubject)
				}
				if errors.Is(err, repository.ErrMemoryTombstoned) || errors.Is(err, repository.ErrNotFound) {
					conflicts++
					reason := "mutation target was not found"
					if errors.Is(err, repository.ErrMemoryTombstoned) {
						reason = "mutation target is tombstoned"
					}
					mutationResults = append(mutationResults, rejectedMutation(mutation, req.Project, reason))
					continue
				}
				return nil, err
			}
			if result == nil {
				return nil, fmt.Errorf("mutation %q returned no result", mutation.EventID)
			}
			terminalFlags := 0
			for _, terminal := range []bool{result.Applied, result.Duplicate, result.Rejected} {
				if terminal {
					terminalFlags++
				}
			}
			if terminalFlags != 1 {
				return nil, fmt.Errorf("mutation %q result must have exactly one terminal flag", mutation.EventID)
			}
			mutationResults = append(mutationResults, *result)
			if result.Applied {
				pushed++
			}
			if result.Rejected {
				conflicts++
				logRejectedMutation(result.EventID, result.Op, req.Project, result.Reason)
			}
		}

		cursor := model.MutationCursor{}
		if req.MutationCursor != nil {
			cursor = *req.MutationCursor
		}
		batch, err := repos.Memory.ListMemoryMutations(ctx, req.Project, cursor, syncMutationPullBatchSize)
		if err != nil {
			return nil, err
		}
		if batch != nil {
			pulledMutations = deliverableMutations(batch.Events, req.Project, req.SyncCapabilities)
			next := batch.Next
			nextMutationCursor = &next
		}
	} else if mutationProtocolCapable {
		compatibilityMode = model.CompatibilityModeLegacy
	}

	resp := &model.SyncResponse{
		Pushed:                 pushed,
		Conflicts:              conflicts,
		PromptsPushed:          promptsPushed,
		PulledMutations:        pulledMutations,
		MutationResults:        mutationResults,
		NextMutationCursor:     nextMutationCursor,
		CompatibilityMode:      compatibilityMode,
		ProjectIdentityVersion: projectidentity.ContractVersion,
		SyncCapabilities:       model.ServerSyncCapabilities(),
	}
	if err := s.emitSyncAudit(ctx, repos.Audit, req.Project, userID, pushed, conflicts, promptsPushed); err != nil {
		return nil, err
	}
	return resp, nil
}

// deliverableMutations withholds the events this client has not declared it can
// apply.
//
// The daemon's apply loop returns a hard error on an op it does not know, which
// aborts the batch before it advances its mutation cursor and before it acks the
// mutations it just pushed — so a single undeliverable event silently and
// permanently stops that daemon from receiving its teammates' work. The server
// is the only side that can prevent it, and the only thing it needs is the
// client's own declaration.
//
// The filter runs here rather than in the SQL: the cursor MUST keep advancing
// over a withheld event, or the daemon would stall on it forever instead of
// merely missing it. batch.Next already points past every row the page scanned,
// so dropping events after the fact keeps the stream moving.
//
// The withheld event is not queued for later. A client that upgrades starts from
// its stored cursor, which is past these events; a reproject it missed reaches it
// as the ordinary memory row under its new project, since applyReprojectMutation
// bumps synced_at. That is a weaker guarantee than replay, and it is the one an
// un-upgraded daemon can actually be given.
//
// Every withheld event is logged, because withholding is silent everywhere else:
// the daemon reports a clean sync, ListActivityFeed filters reproject out by op,
// and emitSyncAudit counts only memories, conflicts and prompts. A client that
// declares its capability with the wrong spelling would otherwise lose every
// reproject forever while both ends reported health. The server is the only side
// that knows both what it held back and what the client declared, so the line
// carries both.
func deliverableMutations(events []model.MutationEnvelope, project string, capabilities []string) []model.MutationEnvelope {
	deliverable := make([]model.MutationEnvelope, 0, len(events))
	for _, event := range events {
		if model.ClientUnderstandsMutationOp(event.Op, capabilities) {
			deliverable = append(deliverable, event)
			continue
		}
		// event_id is quoted for the reason given on logRejectedMutation: it is
		// unvalidated wire input, and this line is the only notice that
		// propagation stopped for this client.
		log.Printf("warn: withheld mutation event_id=%q op=%q project=%q required_capability=%q declared_capabilities=%q",
			event.EventID, event.Op, project, model.MutationOpCapability(event.Op), capabilities)
	}
	return deliverable
}

// rejectedMutation builds a rejection result and logs it in one place, so no
// rejection path can be added without its log line.
func rejectedMutation(mutation model.MutationEnvelope, project, reason string) model.MutationApplyResult {
	logRejectedMutation(mutation.EventID, mutation.Op, project, reason)
	return model.MutationApplyResult{
		EventID:  mutation.EventID,
		Op:       mutation.Op,
		Rejected: true,
		Reason:   reason,
	}
}

// logRejectedMutation records what the server discarded and why.
//
// A rejection is more destructive than it looks: the daemon's
// MarkMutationsRejected drops the event from its local journal and never
// surfaces result.Reason to a human, so the event and its explanation are gone
// the moment the response is processed. Soft-rejecting an op the server does not
// know is still right — the old hard error poisoned the entire batch — but it
// makes this the only surviving record of the loss.
//
// The reason goes last and unquoted: it is free-form prose that already carries
// its own quotes (`unsupported memory mutation op "…"`), and %q would escape
// them into unreadability. Being the final field, it needs no delimiter. That is
// safe only because every reason is server-authored and every attacker-derived
// part it interpolates is already %q-wrapped at the point it is built — see
// rejectedMutationResult's callers in the memory repository.
//
// event_id is quoted for the opposite reason. It is wire input with no binding
// tag and no validation beyond `!= ""`, and this rejection path is reached
// entirely in memory by mutationProjectMismatch, so the uuid column type never
// sees it. Printed raw, a client could embed a newline plus its own well-formed
// `warn: rejected mutation …` line naming someone else's project and have the
// server emit it. %q runs the value through strconv.Quote, which escapes \n, \r,
// NUL, and the other control characters — including U+0085 and U+2028, which
// some log readers also treat as line breaks — so the whole id stays on one
// line, escaped rather than dropped and still greppable.
func logRejectedMutation(eventID string, op model.MutationOp, project, reason string) {
	log.Printf("warn: rejected mutation event_id=%q op=%q project=%q reason: %s", eventID, op, project, reason)
}

func syncRequestProjects(req model.SyncRequest) []string {
	projects := []string{req.Project}
	// A session or prompt push that names a from_project is a relocation and
	// names two projects, exactly like a reproject: the source belongs here or
	// the quarantine precheck would see only the end the row is moving INTO, and
	// the push would carry rows out of a quarantine the request never mentions.
	for _, payload := range req.Sessions {
		projects = append(projects, payload.Project, payload.FromProject)
	}
	for _, payload := range req.Memories {
		projects = append(projects, payload.Project)
	}
	for _, payload := range req.Prompts {
		projects = append(projects, payload.Project, payload.FromProject)
	}
	for _, mutation := range req.Mutations {
		projects = append(projects, mutation.Project)
		if mutation.Memory != nil {
			projects = append(projects, mutation.Memory.Project)
		}
		// A reproject names two projects and the envelope carries only one of
		// them. Both ends belong here: this list is what the quarantine
		// precheck and the project-key lock are built from, so omitting the
		// source would let a reproject carry rows out of a quarantined project,
		// and would take the lock on only half the projects it writes.
		if mutation.Reproject != nil {
			projects = append(projects, mutation.Reproject.FromProject, mutation.Reproject.ToProject)
		}
	}
	return projects
}

// distinctProjects preserves every project literal a request carries, deduped
// in first-seen order. It deliberately does NOT canonicalize: the daemon is the
// sole authority on project identity, and each literal is looked up against the
// literal a block row stores.
//
// repository.ProjectLockKeys dedupes the same literals for the advisory lock and
// SORTS them instead. The orderings differ on purpose: sorting is what stops two
// overlapping transactions deadlocking on the same locks, while first-seen order
// is what makes the rejection here name the projects in the order the request
// presented them. Neither ordering is safe in the other's place.
func distinctProjects(projects []string) []string {
	seen := make(map[string]struct{}, len(projects))
	distinct := make([]string, 0, len(projects))
	for _, project := range projects {
		if strings.TrimSpace(project) == "" {
			continue
		}
		if _, ok := seen[project]; ok {
			continue
		}
		seen[project] = struct{}{}
		distinct = append(distinct, project)
	}
	return distinct
}

func (s *syncService) precheckBlockedProjects(ctx context.Context, req model.SyncRequest, blockRepo repository.ProjectBlockRepository) error {
	if blockRepo == nil {
		return nil
	}
	// Look the block up under the literal an admin quarantined. Folding the
	// request project to a canonical key asked about a project nobody blocked,
	// found nothing, and let the push straight through the quarantine.
	for _, project := range distinctProjects(syncRequestProjects(req)) {
		block, err := blockRepo.GetByProjectKey(ctx, project)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return err
		}
		if block != nil {
			return projectBlockedErrorDelivery(ctx, blockRepo, block, req.AckSubject)
		}
	}
	return nil
}

func mutationProjectMismatch(mutation model.MutationEnvelope, requestProject string) bool {
	if mutation.Project != requestProject {
		return true
	}
	if mutation.Memory != nil && mutation.Memory.Project != "" && mutation.Memory.Project != requestProject {
		return true
	}
	return false
}

func (s *syncService) emitSyncAudit(ctx context.Context, auditRepo repository.AuditRepository, project, userID string, pushed, conflicts, promptsPushed int) error {
	if auditRepo == nil {
		return nil
	}

	for _, entry := range buildSyncAuditEntries(project, userID, pushed, conflicts, promptsPushed) {
		if err := auditRepo.Insert(ctx, entry); err != nil {
			return fmt.Errorf("record sync audit event action=%q project=%q: %w", entry.Action, project, err)
		}
	}
	return nil
}

// PullAll devuelve sesiones y memorias actualizadas después de 'since'.
// Sessions se resuelven PRIMERO — el daemon receptor debe insertarlas antes que las memorias.
//
// limit y los cursores se forwardean tal cual a los repositorios — ver el
// contrato completo en la doc del método de la interfaz SyncService.PullAll.
func (s *syncService) PullAll(ctx context.Context, project string, since time.Time, excludeSyncIDs []string, limit int, memoriesCursor, sessionsCursor model.PullCursor) (*model.PullResult, error) {
	return s.pullAllWithRepos(ctx, syncPullRepos{Memory: s.repo, Session: s.sessionRepo}, project, since, excludeSyncIDs, limit, memoriesCursor, sessionsCursor)
}

func (s *syncService) pullAllWithRepos(ctx context.Context, repos syncPullRepos, project string, since time.Time, excludeSyncIDs []string, limit int, memoriesCursor, sessionsCursor model.PullCursor) (*model.PullResult, error) {
	sessions, sessionsHasMore, err := repos.Session.ListSessionsSince(ctx, project, since, sessionsCursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list sessions since: %w", err)
	}

	memories, memoriesHasMore, err := repos.Memory.PullSince(ctx, project, since, excludeSyncIDs, memoriesCursor, limit)
	if err != nil {
		return nil, fmt.Errorf("pull memories since: %w", err)
	}

	if sessions == nil {
		sessions = []*model.Session{}
	}
	if memories == nil {
		memories = []*model.Memory{}
	}

	result := &model.PullResult{
		Sessions:        sessions,
		Memories:        memories,
		MemoriesHasMore: memoriesHasMore,
		SessionsHasMore: sessionsHasMore,
	}

	if memoriesHasMore && len(memories) > 0 {
		last := memories[len(memories)-1]
		result.NextPullCursor = &model.PullCursor{SyncedAt: last.SyncedAt, SyncID: last.SyncID}
	}

	if sessionsHasMore && len(sessions) > 0 {
		last := sessions[len(sessions)-1]
		result.NextSessionCursor = &model.PullCursor{SyncedAt: last.SyncedAt, SyncID: last.SyncID}
	}

	return result, nil
}

func (s *syncService) syncResponseWithPull(ctx context.Context, req model.SyncRequest, pushResp *model.SyncResponse, repos syncPullRepos) (*model.SyncResponse, error) {
	var since time.Time
	if req.LastSync != nil {
		since = *req.LastSync
	}
	excludeIDs := make([]string, 0, len(req.Memories))
	for _, m := range req.Memories {
		excludeIDs = append(excludeIDs, m.SyncID)
	}
	var memoriesCursor, sessionsCursor model.PullCursor
	if req.PullCursor != nil {
		memoriesCursor = *req.PullCursor
	}
	if req.PullSessionCursor != nil {
		sessionsCursor = *req.PullSessionCursor
	}
	pullResult, err := s.pullAllWithRepos(ctx, repos, req.Project, since, excludeIDs, model.ClampPullLimit(req.PullLimit), memoriesCursor, sessionsCursor)
	if err != nil {
		return nil, err
	}
	pulledSessions := make([]model.SyncSessionResponse, 0, len(pullResult.Sessions))
	for _, sess := range pullResult.Sessions {
		pulledSessions = append(pulledSessions, model.SyncSessionResponse{ID: sess.ID, SyncID: sess.SyncID, Project: sess.Project, Directory: sess.Directory, DevID: sess.DevID, Client: sess.Client, StartedAt: sess.StartedAt, EndedAt: sess.EndedAt, Summary: sess.Summary})
	}
	pulled := pullResult.Memories
	if pulled == nil {
		pulled = []*model.Memory{}
	}
	return &model.SyncResponse{
		Pushed:                 pushResp.Pushed,
		Pulled:                 pulled,
		Conflicts:              pushResp.Conflicts,
		PromptsPushed:          pushResp.PromptsPushed,
		PulledSessions:         pulledSessions,
		NextMutationCursor:     pushResp.NextMutationCursor,
		PulledMutations:        pushResp.PulledMutations,
		MutationResults:        pushResp.MutationResults,
		CompatibilityMode:      pushResp.CompatibilityMode,
		ProjectIdentityVersion: pushResp.ProjectIdentityVersion,
		SyncCapabilities:       pushResp.SyncCapabilities,
		PulledHasMore:          pullResult.MemoriesHasMore,
		NextPullCursor:         pullResult.NextPullCursor,
		PulledSessionsHasMore:  pullResult.SessionsHasMore,
		NextSessionCursor:      pullResult.NextSessionCursor,
	}, nil
}

// resolveSessionID resolves the effective session_id for a memory being pushed.
//
// Rules (T4.4 + T4.5 + T4.11):
//  1. session_id absent → lazy-create manual-save-{project} (backward compat).
//  2. session_id is manual-save-* → lazy-create it via EnsureManualSaveSession.
//  3. session_id is present in the payload's sessions[] → already upserted, use it.
//  4. session_id is non-empty, NOT in payload → check DB; if absent → reject (422).
//
// T4.11: legacy-pre-lifecycle-* ids are NOT treated as lazy-create targets.
// They are real session ids that should exist (migration creates them).
// If present in the payload (branch 3), they are used directly.
// If not in the payload, they fall through to the DB check (branch 4).
func (s *syncService) resolveSessionID(ctx context.Context, sessionRepo repository.SessionRepository, sessionID, project string, inPayload map[string]bool) (*string, error) {
	if sessionID == "" {
		// T4.4: no session_id — old daemon or missing field; lazy-create manual-save-*
		log.Printf("warn: memory pushed without session_id for project %q — creating manual-save session", project)
	}

	// inPayload short-circuit: a session arriving in this push was already upserted
	// in Fase 1, so the FK will be satisfied even if it's not yet in the DB at the
	// moment of this validation. We still must check project attribution.
	if inPayload[sessionID] {
		// We trust that Fase 1 inserted with the project attribute the daemon
		// asserted. resolveSessionID never re-reads the row to assert match —
		// the daemon's payload is the source of truth for in-payload sessions.
		return &sessionID, nil
	}

	// R3-FIX-2 — delegate to shared validator (used also by memoryService.Create).
	resolved, err := validateSessionAttribution(ctx, sessionRepo, sessionID, project)
	if err != nil {
		return nil, err
	}
	return &resolved, nil
}
