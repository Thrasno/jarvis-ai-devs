package service

import (
	"context"
	"errors"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/repository"
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
	SetLevel(ctx context.Context, username string, newLevel model.UserLevel) error

	// Deactivate deshabilita un usuario (is_active = false).
	// Devuelve repository.ErrNotFound si el usuario no existe.
	Deactivate(ctx context.Context, username string) error
}

type adminService struct {
	userRepo repository.UserRepository
	memRepo  repository.MemoryRepository
}

// NewAdminService crea el AdminService con los repositorios inyectados.
// Inyectamos memRepo aunque aún no lo usemos — lo necesitaremos para estadísticas.
func NewAdminService(userRepo repository.UserRepository, memRepo repository.MemoryRepository) AdminService {
	return &adminService{
		userRepo: userRepo,
		memRepo:  memRepo,
	}
}

func (s *adminService) ListUsers(ctx context.Context) ([]*model.User, error) {
	return s.userRepo.List(ctx)
}

// SetLevel implementa la lógica de cambio de nivel con la regla de 3 admins.
//
// La lógica de verificación del límite es:
//   - Si el nuevo nivel NO es admin → no necesitamos contar (podemos proceder)
//   - Si el usuario YA es admin y le asignamos admin → no incrementamos el conteo (podemos proceder)
//   - Si el nuevo nivel ES admin y el usuario NO era admin → verificar que haya cupo
func (s *adminService) SetLevel(ctx context.Context, username string, newLevel model.UserLevel) error {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return err
	}

	// Solo necesitamos verificar el límite si estamos SUBIENDO a admin
	// (cuando newLevel es admin Y el usuario actualmente no es admin).
	if newLevel == model.LevelAdmin && user.Level != model.LevelAdmin {
		count, err := s.userRepo.CountAdmins(ctx)
		if err != nil {
			return err
		}
		if count >= maxAdmins {
			return ErrMaxAdminsReached
		}
	}

	return s.userRepo.UpdateLevel(ctx, user.ID, newLevel)
}

func (s *adminService) Deactivate(ctx context.Context, username string) error {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return err
	}
	return s.userRepo.Deactivate(ctx, user.ID)
}
