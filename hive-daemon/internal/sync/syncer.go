package sync

import (
	"context"
	"fmt"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
)

// SyncStore define los métodos del DB que necesita el Syncer.
// *db.DB los implementa todos.
type SyncStore interface {
	GetUnsynced(project string) ([]*models.Memory, error)
	MarkSynced(syncID string, at time.Time) error
	SaveFromRemote(mem *models.Memory) error
	GetLastSync(project string) (time.Time, error)
	SetLastSync(project string, at time.Time) error
	GetJWT() string
	SetJWT(token string, expiresAt time.Time) error
}

// Result resume los resultados de un sync.
type Result struct {
	Pushed    int
	Pulled    int
	Conflicts int
	Project   string
}

// Syncer orquesta el ciclo completo de sincronización para un proyecto.
type Syncer struct {
	store  SyncStore
	client *client
}

// New crea un Syncer con las dependencias inyectadas.
func New(cfg *Config, store SyncStore) *Syncer {
	return &Syncer{
		store:  store,
		client: newClient(cfg),
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
	// Paso 1: JWT
	token, err := s.getOrRefreshToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("autenticación: %w", err)
	}

	// Paso 2: memorias locales pendientes de sync
	unsynced, err := s.store.GetUnsynced(project)
	if err != nil {
		return nil, fmt.Errorf("obtener memorias no sincronizadas: %w", err)
	}

	// Paso 3 + 4: sync bidireccional con el servidor
	lastSync, _ := s.store.GetLastSync(project)
	var lastSyncPtr *time.Time
	if !lastSync.IsZero() {
		lastSyncPtr = &lastSync
	}

	resp, err := s.client.sync(ctx, token, project, unsynced, lastSyncPtr)
	if err != nil {
		return nil, fmt.Errorf("sync con servidor: %w", err)
	}

	// Paso 5a: marcamos como sincronizadas las que enviamos
	now := time.Now()
	for _, m := range unsynced {
		if err := s.store.MarkSynced(m.SyncID, now); err != nil {
			// No abortamos — mejor tener datos duplicados que perder el sync
			// En el próximo sync, el servidor los rechazará por sync_id duplicado
			_ = err
		}
	}

	// Paso 5b: guardamos las memorias que nos mandó el servidor
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
		}
		_ = s.store.SaveFromRemote(mem) // errores no son fatales
	}

	// Paso 6: actualizamos el timestamp del último sync exitoso
	_ = s.store.SetLastSync(project, now)

	return &Result{
		Pushed:    resp.Pushed,
		Pulled:    len(resp.Pulled),
		Conflicts: resp.Conflicts,
		Project:   project,
	}, nil
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
