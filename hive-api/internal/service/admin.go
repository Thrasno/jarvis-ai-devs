package service

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

// ErrMaxAdminsReached se devuelve cuando intentamos ascender a admin
// pero ya hay 3 admins en el sistema.
// El handler lo mapea a HTTP 409 Conflict con mensaje explicativo.
var ErrMaxAdminsReached = errors.New("máximo de admins alcanzado (límite: 3)")

// ErrInsufficientAdmins is returned when deactivating an admin would leave no
// active admin account available to manage the system.
var ErrInsufficientAdmins = errors.New("insufficient active admins remaining")

var ErrSelfAdminMutation = errors.New("cannot mutate your own admin account")

// maxAdmins es el número máximo de administradores permitidos en el sistema.
// Es una constante de negocio — si el equipo crece, se puede cambiar aquí.
const maxAdmins = 3

const auditBestEffortTimeout = 2 * time.Second

// AdminService gestiona las operaciones de administración del sistema.
type AdminService interface {
	// ListUsers devuelve todos los usuarios registrados.
	ListUsers(ctx context.Context) ([]model.AdminUserResponse, error)

	// SetLevel cambia el nivel de acceso de un usuario identificado por username.
	// Si newLevel es LevelAdmin, verifica que no se supere el límite de 3 admins.
	// Devuelve ErrMaxAdminsReached si el límite está alcanzado.
	// Devuelve repository.ErrNotFound si el usuario no existe.
	SetLevel(ctx context.Context, actor model.AdminActor, username string, newLevel model.UserLevel) error

	CreateUser(ctx context.Context, actor model.AdminActor, req model.CreateUserRequest) error
	ResetTemporaryPassword(ctx context.Context, actor model.AdminActor, username string, req model.ResetTemporaryPasswordRequest) error
	Activate(ctx context.Context, actor model.AdminActor, username string) error

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
	syncRepo  repository.SyncAttemptRepository
	now       func() time.Time
}

// NewAdminService crea el AdminService con los repositorios inyectados.
// Inyectamos memRepo aunque aún no lo usemos — lo necesitaremos para estadísticas.
func NewAdminService(userRepo repository.UserRepository, memRepo repository.MemoryRepository, auditRepo repository.AuditRepository, tx repository.TxManager, options ...any) AdminService {
	svc := &adminService{
		userRepo:  userRepo,
		memRepo:   memRepo,
		auditRepo: auditRepo,
		tx:        tx,
		now:       func() time.Time { return time.Now().UTC() },
	}
	if len(options) > 0 {
		svc.syncRepo, _ = options[0].(repository.SyncAttemptRepository)
	}
	if len(options) > 1 {
		if clock, ok := options[1].(func() time.Time); ok {
			svc.now = clock
		}
	}
	return svc
}

func (s *adminService) ListUsers(ctx context.Context) ([]model.AdminUserResponse, error) {
	users, err := s.userRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	rows := map[string]model.UserSyncProjectionRow{}
	projectionAvailable := true
	if s.syncRepo != nil {
		projection, projectionErr := s.syncRepo.UserSyncProjection(ctx, now)
		projectionAvailable = projectionErr == nil
		if projectionAvailable {
			for _, row := range projection.Rows {
				rows[row.PortalUserID] = row
			}
		}
	}
	response := make([]model.AdminUserResponse, 0, len(users))
	for _, user := range users {
		row := rows[user.ID]
		row.IsActive = user.IsActive
		status := userSyncStatus(row, now)
		if !projectionAvailable {
			status = model.UserSyncStatusUnknown
		}
		response = append(response, model.AdminUserResponse{ID: user.ID, Username: user.Username, Email: user.Email, Level: user.Level, IsActive: user.IsActive, CreatedAt: user.CreatedAt, SyncStatus: status, LastSyncAt: userSyncLastSyncAt(row)})
	}
	return response, nil
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

func (s *adminService) CreateUser(ctx context.Context, actor model.AdminActor, req model.CreateUserRequest) error {
	passwordHash, err := hashPassword(req.TemporaryPassword)
	if err != nil {
		return err
	}
	err = s.tx.WithinTx(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		if req.Level == model.LevelAdmin {
			if err := repos.Users.LockActiveAdminInvariant(ctx); err != nil {
				return err
			}
			count, err := repos.Users.CountAdmins(ctx)
			if err != nil {
				return err
			}
			if count >= maxAdmins {
				return ErrMaxAdminsReached
			}
		}
		created, err := repos.Users.Create(ctx, &model.User{
			Username: req.Username,
			Email:    req.Email,
			Password: passwordHash,
			Level:    req.Level,
			IsActive: true,
		})
		if err != nil {
			return err
		}
		return repos.Audit.Insert(ctx, buildUserCreateAudit(actor, created))
	})
	if err != nil {
		s.insertAuditBestEffort(ctx, buildUserCreateFailureAudit(actor, req, err))
	}
	return err
}

func (s *adminService) ResetTemporaryPassword(ctx context.Context, actor model.AdminActor, username string, req model.ResetTemporaryPasswordRequest) error {
	if actor.Username == username {
		err := ErrSelfAdminMutation
		s.insertAuditBestEffort(ctx, buildUserPasswordResetFailureAudit(actor, username, nil, err))
		return err
	}
	passwordHash, err := hashPassword(req.TemporaryPassword)
	if err != nil {
		return err
	}
	var target *model.User
	err = s.tx.WithinTx(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		user, err := repos.Users.GetByUsername(ctx, username)
		if err != nil {
			return err
		}
		target = user
		if err := repos.Users.UpdatePassword(ctx, user.ID, passwordHash); err != nil {
			return err
		}
		return repos.Audit.Insert(ctx, buildUserPasswordResetAudit(actor, user))
	})
	if err != nil {
		s.insertAuditBestEffort(ctx, buildUserPasswordResetFailureAudit(actor, username, target, err))
	}
	return err
}

func (s *adminService) Activate(ctx context.Context, actor model.AdminActor, username string) error {
	if actor.Username == username {
		err := ErrSelfAdminMutation
		s.insertAuditBestEffort(ctx, buildUserActivateFailureAudit(actor, username, nil, err))
		return err
	}
	var target *model.User
	err := s.tx.WithinTx(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		user, err := repos.Users.GetByUsername(ctx, username)
		if err != nil {
			return err
		}
		target = user
		if !user.IsActive {
			if err := repos.Users.LockActiveAdminInvariant(ctx); err != nil {
				return err
			}
			user, err = repos.Users.GetByUsername(ctx, username)
			if err != nil {
				return err
			}
			target = user
		}
		if !user.IsActive && user.Level == model.LevelAdmin {
			count, err := repos.Users.CountAdmins(ctx)
			if err != nil {
				return err
			}
			if count >= maxAdmins {
				return ErrMaxAdminsReached
			}
		}
		if err := repos.Users.Activate(ctx, user.ID); err != nil {
			return err
		}
		return repos.Audit.Insert(ctx, buildUserActivateAudit(actor, user))
	})
	if err != nil {
		s.insertAuditBestEffort(ctx, buildUserActivateFailureAudit(actor, username, target, err))
	}
	return err
}

// SetLevel implementa la lógica de cambio de nivel con la regla de 3 admins.
func (s *adminService) SetLevel(ctx context.Context, actor model.AdminActor, username string, newLevel model.UserLevel) error {
	if actor.Username == username {
		err := ErrSelfAdminMutation
		s.insertAuditBestEffort(ctx, buildUserLevelChangeFailureAudit(actor, username, nil, newLevel, err))
		return err
	}
	var target *model.User
	err := s.tx.WithinTx(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		user, err := repos.Users.GetByUsername(ctx, username)
		if err != nil {
			return err
		}
		target = user

		if newLevel == model.LevelAdmin && user.Level != model.LevelAdmin {
			if err := repos.Users.LockActiveAdminInvariant(ctx); err != nil {
				return err
			}
			user, err = repos.Users.GetByUsername(ctx, username)
			if err != nil {
				return err
			}
			target = user
			if user.IsActive && user.Level != model.LevelAdmin {
				count, err := repos.Users.CountAdmins(ctx)
				if err != nil {
					return err
				}
				if count >= maxAdmins {
					return ErrMaxAdminsReached
				}
			}
		}
		if user.IsActive && user.Level == model.LevelAdmin && newLevel != model.LevelAdmin {
			if err := repos.Users.LockActiveAdminInvariant(ctx); err != nil {
				return err
			}
			count, err := repos.Users.CountAdmins(ctx)
			if err != nil {
				return err
			}
			if count <= 1 {
				return ErrInsufficientAdmins
			}
		}

		if err := repos.Users.UpdateLevel(ctx, user.ID, newLevel); err != nil {
			return err
		}
		return repos.Audit.Insert(ctx, buildUserLevelChangeAudit(actor, user, newLevel))
	})
	if err != nil {
		s.insertAuditBestEffort(ctx, buildUserLevelChangeFailureAudit(actor, username, target, newLevel, err))
	}
	return err
}

func (s *adminService) Deactivate(ctx context.Context, actor model.AdminActor, username string) error {
	if actor.Username == username {
		err := ErrSelfAdminMutation
		s.insertAuditBestEffort(ctx, buildUserDeactivateFailureAudit(actor, username, nil, err))
		return err
	}
	var target *model.User
	err := s.tx.WithinTx(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		user, err := repos.Users.GetByUsername(ctx, username)
		if err != nil {
			return err
		}
		target = user
		if user.IsActive && user.Level == model.LevelAdmin {
			if err := repos.Users.LockActiveAdminInvariant(ctx); err != nil {
				return err
			}
			count, err := repos.Users.CountAdmins(ctx)
			if err != nil {
				return err
			}
			if count <= 1 {
				return ErrInsufficientAdmins
			}
		}
		if err := repos.Users.Deactivate(ctx, user.ID); err != nil {
			return err
		}
		return repos.Audit.Insert(ctx, buildUserDeactivateAudit(actor, user))
	})
	if err != nil {
		s.insertAuditBestEffort(ctx, buildUserDeactivateFailureAudit(actor, username, target, err))
	}
	return err
}

func hashPassword(password string) (string, error) {
	if err := model.ValidateTemporaryPasswordBytes(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (s *adminService) insertAuditBestEffort(_ context.Context, entry *model.AuditEntry) {
	if entry == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), auditBestEffortTimeout)
	defer cancel()
	if err := s.auditRepo.Insert(ctx, entry); err != nil {
		reasonCode := ""
		if entry.ReasonCode != nil {
			reasonCode = *entry.ReasonCode
		}
		log.Printf("warn: failed to insert admin audit entry action=%q outcome=%q reason_code=%q: %v", entry.Action, entry.Outcome, reasonCode, err)
	}
}

// GrantAdmin asciende a admin con idempotencia y verificación del límite.
// A diferencia de SetLevel (que acepta cualquier nivel), GrantAdmin es específico
// para el ascenso a admin — hace la comprobación del límite siempre, salvo que
// el usuario ya sea admin (en cuyo caso es un no-op seguro).
func (s *adminService) GrantAdmin(ctx context.Context, actor model.AdminActor, username string) error {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		s.insertAuditBestEffort(ctx, buildUserLevelChangeFailureAudit(actor, username, nil, model.LevelAdmin, err))
		return err
	}
	target := user

	// Idempotente: ya es admin → retornamos sin error y sin tocar la BD.
	if user.Level == model.LevelAdmin {
		return nil
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		if err := repos.Users.LockActiveAdminInvariant(ctx); err != nil {
			return err
		}
		user, err = repos.Users.GetByUsername(ctx, username)
		if err != nil {
			return err
		}
		target = user
		if user.Level == model.LevelAdmin {
			return nil
		}
		if user.IsActive {
			count, err := repos.Users.CountAdmins(ctx)
			if err != nil {
				return err
			}
			if count >= maxAdmins {
				return ErrMaxAdminsReached
			}
		}
		if err := repos.Users.UpdateLevel(ctx, user.ID, model.LevelAdmin); err != nil {
			return err
		}
		return repos.Audit.Insert(ctx, buildUserLevelChangeAudit(actor, user, model.LevelAdmin))
	})
	if err != nil {
		s.insertAuditBestEffort(ctx, buildUserLevelChangeFailureAudit(actor, username, target, model.LevelAdmin, err))
	}
	return err
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

	byProject, err := s.memRepo.CountByProject(ctx, model.MemoryFilter{})
	if err != nil {
		return nil, err
	}
	if byProject == nil {
		byProject = []model.ProjectCount{}
	}
	stats.Memories.ByProject = byProject

	// ByCategory: explicit empty — no query yet
	stats.Memories.ByCategory = []model.CategoryCount{}

	return stats, nil
}
