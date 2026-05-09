package sync

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
)

const (
	backoffBaseDelay = 30 * time.Second
	backoffMaxDelay  = 15 * time.Minute
	backoffJitterPct = 4
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

// Result resume los resultados de un sync.
type Result struct {
	Pushed        int
	Pulled        int
	Conflicts     int
	PromptsPushed int
	Project       string
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
	now    func() time.Time
	jitter func(max time.Duration) time.Duration
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
			return time.Duration(rand.Int63n(int64(max) + 1))
		},
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

	// Paso 2: memorias locales pendientes de sync
	unsynced, err := s.store.GetUnsynced(project)
	if err != nil {
		if recordErr := s.recordFailure(project, health, now, err); recordErr != nil {
			return nil, recordErr
		}
		return nil, fmt.Errorf("obtener memorias no sincronizadas: %w", err)
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

	// Paso 3 + 4: sync bidireccional con el servidor
	lastSync, _ := s.store.GetLastSync(project)
	var lastSyncPtr *time.Time
	if !lastSync.IsZero() {
		lastSyncPtr = &lastSync
	}

	resp, err := s.client.sync(ctx, token, project, unsyncedSessions, unsynced, unsyncedPrompts, lastSyncPtr)
	if err != nil {
		if recordErr := s.recordFailure(project, health, now, err); recordErr != nil {
			return nil, recordErr
		}
		return nil, fmt.Errorf("sync con servidor: %w", err)
	}

	// Paso 5a: marcamos como sincronizadas las sesiones que enviamos
	for _, sess := range unsyncedSessions {
		if err := s.store.MarkSessionSynced(sess.ID, now); err != nil {
			logger.Log.Printf("warn: MarkSessionSynced %s: %v", sess.ID, err)
		}
	}

	// Paso 5b: marcamos como sincronizadas las memorias que enviamos
	for _, m := range unsynced {
		if err := s.store.MarkSynced(m.SyncID, now); err != nil {
			// No abortamos — mejor tener datos duplicados que perder el sync
			// En el próximo sync, el servidor los rechazará por sync_id duplicado
			logger.Log.Printf("warn: MarkSynced %s: %v", m.SyncID, err)
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

	// Paso 6: actualizamos el timestamp del último sync exitoso
	if err := s.store.RecordSyncSuccess(project, now); err != nil {
		return nil, fmt.Errorf("registrar éxito de sync: %w", err)
	}

	return &Result{
		Pushed:        resp.Pushed,
		Pulled:        len(resp.Pulled),
		Conflicts:     resp.Conflicts,
		PromptsPushed: resp.PromptsPushed,
		Project:       project,
	}, nil
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
	return nil
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
