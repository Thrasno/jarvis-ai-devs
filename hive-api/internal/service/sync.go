package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/repository"
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
)

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

	// PullAll devuelve sesiones Y memorias actualizadas después de 'since'.
	// Sessions se devuelven PRIMERO para que el daemon receptor satisfaga la FK.
	PullAll(ctx context.Context, project string, since time.Time, excludeSyncIDs []string) (*model.PullResult, error)
}

type syncService struct {
	repo        repository.MemoryRepository
	promptRepo  repository.PromptRepository
	sessionRepo repository.SessionRepository
	auditRepo   repository.AuditRepository
}

// NewSyncService crea el SyncService con los repositorios inyectados.
// memRepo gestiona memorias; promptRepo gestiona user-prompts;
// sessionRepo gestiona sesiones (requerido para ordering FK en push).
func NewSyncService(memRepo repository.MemoryRepository, promptRepo repository.PromptRepository, sessionRepo repository.SessionRepository, auditRepo repository.AuditRepository) SyncService {
	return &syncService{repo: memRepo, promptRepo: promptRepo, sessionRepo: sessionRepo, auditRepo: auditRepo}
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
			ID:        sp.ID,
			SyncID:    sp.SyncID,
			Project:   sp.Project,
			Directory: sp.Directory,
			DevID:     sp.DevID,
			Client:    sp.Client,
			StartedAt: sp.StartedAt,
			EndedAt:   sp.EndedAt,
			Summary:   sp.Summary,
		}
		if err := s.sessionRepo.UpsertSession(ctx, sess); err != nil {
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
		sessionID, err := s.resolveSessionID(ctx, payload.SessionID, req.Project, inPayloadSessions)
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
			Confidence:    payload.Confidence,
			ImpactScore:   payload.ImpactScore,
			SessionID:     sessionID,
		}

		saved, wasInsert, err := s.repo.Upsert(ctx, mem)
		if err != nil {
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
		p := &model.Prompt{
			SyncID:    payload.SyncID,
			Project:   payload.Project,
			Content:   payload.Content,
			CreatedBy: userID,
			CreatedAt: payload.CreatedAt,
		}
		saved, err := s.promptRepo.Upsert(ctx, p)
		if err != nil {
			return nil, err
		}
		if saved {
			promptsPushed++
		}
	}

	var pulledMutations []model.MutationEnvelope
	var nextMutationCursor *model.MutationCursor
	compatibilityMode := ""
	if mutationProtocolAuthoritative {
		compatibilityMode = model.CompatibilityModeMutationV2
		for _, mutation := range req.Mutations {
			if mutationProjectMismatch(mutation, req.Project) {
				conflicts++
				continue
			}

			result, err := s.repo.ApplyMemoryMutation(ctx, mutation)
			if err != nil {
				if errors.Is(err, repository.ErrMemoryTombstoned) || errors.Is(err, repository.ErrNotFound) {
					conflicts++
					continue
				}
				return nil, err
			}
			if result == nil {
				continue
			}
			if result.Applied {
				pushed++
			}
			if result.Rejected {
				conflicts++
			}
		}

		cursor := model.MutationCursor{}
		if req.MutationCursor != nil {
			cursor = *req.MutationCursor
		}
		batch, err := s.repo.ListMemoryMutations(ctx, req.Project, cursor, 100)
		if err != nil {
			return nil, err
		}
		if batch != nil {
			pulledMutations = batch.Events
			next := batch.Next
			nextMutationCursor = &next
		}
	} else if mutationProtocolCapable {
		compatibilityMode = model.CompatibilityModeLegacy
	}

	resp := &model.SyncResponse{
		Pushed:             pushed,
		Conflicts:          conflicts,
		PromptsPushed:      promptsPushed,
		PulledMutations:    pulledMutations,
		NextMutationCursor: nextMutationCursor,
		CompatibilityMode:  compatibilityMode,
	}
	s.emitSyncAudit(ctx, req.Project, userID, pushed, conflicts, promptsPushed)
	return resp, nil
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

func (s *syncService) emitSyncAudit(ctx context.Context, project, userID string, pushed, conflicts, promptsPushed int) {
	if s.auditRepo == nil {
		return
	}

	for _, entry := range buildSyncAuditEntries(project, userID, pushed, conflicts, promptsPushed) {
		if err := s.auditRepo.Insert(ctx, entry); err != nil {
			log.Printf("warn: failed to insert sync audit event action=%q project=%q: %v", entry.Action, project, err)
		}
	}
}

// PullAll devuelve sesiones y memorias actualizadas después de 'since'.
// Sessions se resuelven PRIMERO — el daemon receptor debe insertarlas antes que las memorias.
func (s *syncService) PullAll(ctx context.Context, project string, since time.Time, excludeSyncIDs []string) (*model.PullResult, error) {
	sessions, err := s.sessionRepo.ListSessionsSince(ctx, project, since)
	if err != nil {
		return nil, fmt.Errorf("list sessions since: %w", err)
	}

	memories, err := s.repo.PullSince(ctx, project, since, excludeSyncIDs)
	if err != nil {
		return nil, fmt.Errorf("pull memories since: %w", err)
	}

	if sessions == nil {
		sessions = []*model.Session{}
	}
	if memories == nil {
		memories = []*model.Memory{}
	}

	return &model.PullResult{Sessions: sessions, Memories: memories}, nil
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
func (s *syncService) resolveSessionID(ctx context.Context, sessionID, project string, inPayload map[string]bool) (*string, error) {
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
	resolved, err := validateSessionAttribution(ctx, s.sessionRepo, sessionID, project)
	if err != nil {
		return nil, err
	}
	return &resolved, nil
}
