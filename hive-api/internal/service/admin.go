package service

import (
	"context"
	"errors"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
)

// ErrMaxAdminsReached se devuelve cuando intentamos ascender a admin
// pero ya hay 3 admins en el sistema.
// El handler lo mapea a HTTP 409 Conflict con mensaje explicativo.
var ErrMaxAdminsReached = errors.New("máximo de admins alcanzado (límite: 3)")

// maxAdmins es el número máximo de administradores permitidos en el sistema.
// Es una constante de negocio — si el equipo crece, se puede cambiar aquí.
const maxAdmins = 3

// AdminService gestiona las operaciones de administración del sistema.
type AdminService interface {
	// ListUsers devuelve todos los usuarios registrados.
	ListUsers(ctx context.Context) ([]*model.User, error)

	// SetLevel cambia el nivel de acceso de un usuario identificado por username.
	// Si newLevel es LevelAdmin, verifica que no se supere el límite de 3 admins.
	// Devuelve ErrMaxAdminsReached si el límite está alcanzado.
	// Devuelve repository.ErrNotFound si el usuario no existe.
	SetLevel(ctx context.Context, actor model.AdminActor, username string, newLevel model.UserLevel) error

	// GrantAdmin asciende a un usuario a nivel admin.
	// Idempotente: si ya es admin, devuelve nil sin error.
	// Devuelve ErrMaxAdminsReached si ya hay 3 admins.
	// Devuelve repository.ErrNotFound si el usuario no existe.
	GrantAdmin(ctx context.Context, actor model.AdminActor, username string) error

	// Deactivate deshabilita un usuario (is_active = false).
	// Devuelve repository.ErrNotFound si el usuario no existe.
	Deactivate(ctx context.Context, actor model.AdminActor, username string) error

	// GetStats devuelve estadísticas agregadas del sistema.
	GetStats(ctx context.Context) (*model.AdminStatsResponse, error)

	// ListAuditLogs devuelve auditoría paginada para consumo admin.
	ListAuditLogs(ctx context.Context, filter model.AuditFilter) (model.AuditListResponse, error)
}

type adminService struct {
	userRepo  repository.UserRepository
	memRepo   repository.MemoryRepository
	auditRepo repository.AuditRepository
	tx        repository.TxManager
}

// NewAdminService crea el AdminService con los repositorios inyectados.
// Inyectamos memRepo aunque aún no lo usemos — lo necesitaremos para estadísticas.
func NewAdminService(userRepo repository.UserRepository, memRepo repository.MemoryRepository, auditRepo repository.AuditRepository, tx repository.TxManager) AdminService {
	return &adminService{
		userRepo:  userRepo,
		memRepo:   memRepo,
		auditRepo: auditRepo,
		tx:        tx,
	}
}

func (s *adminService) ListUsers(ctx context.Context) ([]*model.User, error) {
	return s.userRepo.List(ctx)
}

func (s *adminService) ListAuditLogs(ctx context.Context, filter model.AuditFilter) (model.AuditListResponse, error) {
	filter = filter.Normalize()
	entries, err := s.auditRepo.List(ctx, filter)
	if err != nil {
		return model.AuditListResponse{}, err
	}
	total, err := s.auditRepo.Count(ctx, filter)
	if err != nil {
		return model.AuditListResponse{}, err
	}
	return model.NewAuditListResponse(entries, total, filter), nil
}

// SetLevel implementa la lógica de cambio de nivel con la regla de 3 admins.
//
// La lógica de verificación del límite es:
//   - Si el nuevo nivel NO es admin → no necesitamos contar (podemos proceder)
//   - Si el usuario YA es admin y le asignamos admin → no incrementamos el conteo (podemos proceder)
//   - Si el nuevo nivel ES admin y el usuario NO era admin → verificar que haya cupo
func (s *adminService) SetLevel(ctx context.Context, actor model.AdminActor, username string, newLevel model.UserLevel) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		user, err := repos.Users.GetByUsername(ctx, username)
		if err != nil {
			return err
		}

		// Solo necesitamos verificar el límite si estamos SUBIENDO a admin
		// (cuando newLevel es admin Y el usuario actualmente no es admin).
		if newLevel == model.LevelAdmin && user.Level != model.LevelAdmin {
			count, err := repos.Users.CountAdmins(ctx)
			if err != nil {
				return err
			}
			if count >= maxAdmins {
				return ErrMaxAdminsReached
			}
		}

		if err := repos.Users.UpdateLevel(ctx, user.ID, newLevel); err != nil {
			return err
		}
		return repos.Audit.Insert(ctx, buildUserLevelChangeAudit(actor, user, newLevel))
	})
}

func (s *adminService) Deactivate(ctx context.Context, actor model.AdminActor, username string) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		user, err := repos.Users.GetByUsername(ctx, username)
		if err != nil {
			return err
		}
		if err := repos.Users.Deactivate(ctx, user.ID); err != nil {
			return err
		}
		return repos.Audit.Insert(ctx, buildUserDeactivateAudit(actor, user))
	})
}

// GrantAdmin asciende a admin con idempotencia y verificación del límite.
// A diferencia de SetLevel (que acepta cualquier nivel), GrantAdmin es específico
// para el ascenso a admin — hace la comprobación del límite siempre, salvo que
// el usuario ya sea admin (en cuyo caso es un no-op seguro).
func (s *adminService) GrantAdmin(ctx context.Context, actor model.AdminActor, username string) error {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return err
	}

	// Idempotente: ya es admin → retornamos sin error y sin tocar la BD.
	if user.Level == model.LevelAdmin {
		return nil
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		count, err := repos.Users.CountAdmins(ctx)
		if err != nil {
			return err
		}
		if count >= maxAdmins {
			return ErrMaxAdminsReached
		}
		if err := repos.Users.UpdateLevel(ctx, user.ID, model.LevelAdmin); err != nil {
			return err
		}
		return repos.Audit.Insert(ctx, buildUserLevelChangeAudit(actor, user, model.LevelAdmin))
	})
}

// GetStats recopila estadísticas agregadas de usuarios y memorias.
// Para el MVP usamos métodos existentes del repo + agregación en Go.
func (s *adminService) GetStats(ctx context.Context) (*model.AdminStatsResponse, error) {
	users, err := s.userRepo.List(ctx)
	if err != nil {
		return nil, err
	}

	// Agregamos estadísticas de usuarios en Go — evita una query SQL extra
	stats := &model.AdminStatsResponse{}
	stats.Users.Total = len(users)
	stats.Users.ByLevel = map[string]int{
		string(model.LevelViewer): 0,
		string(model.LevelMember): 0,
		string(model.LevelAdmin):  0,
	}
	for _, u := range users {
		if u.IsActive {
			stats.Users.Active++
		}
		stats.Users.ByLevel[string(u.Level)]++
	}

	// Total de memorias
	total, err := s.memRepo.Count(ctx, model.MemoryFilter{})
	if err != nil {
		return nil, err
	}
	stats.Memories.Total = total

	// Arrays vacíos explícitos — nunca null en JSON (spec: zeros as 0 not null)
	stats.Memories.ByProject = []model.ProjectCount{}
	stats.Memories.ByCategory = []model.CategoryCount{}

	return stats, nil
}
