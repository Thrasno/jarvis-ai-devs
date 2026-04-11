package repository

// Este archivo define un mock de UserRepository para usar en tests.
//
// Los mocks viven en el mismo paquete que las interfaces para que cualquier
// test del proyecto pueda importarlos con:
//   import "github.com/Thrasno/jarvis-dev/hive-api/internal/repository"
//   mockRepo := &repository.MockUserRepository{}
//
// No tienen el sufijo _test.go porque necesitan ser importables desde
// otros paquetes de test (como internal/service/).

import (
	"context"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/stretchr/testify/mock"
)

// MockUserRepository es una implementación falsa de UserRepository.
// Implementa todos los métodos de la interfaz — el compilador lo verifica.
//
// Embebe mock.Mock, que es la "caja de herramientas" de testify:
// registra qué métodos se llamaron, con qué argumentos, y qué devolver.
type MockUserRepository struct {
	mock.Mock
}

// Verificación en tiempo de compilación: si MockUserRepository no implementa
// UserRepository, este programa no compilará.
// Es una forma de documentar la intención y atrapar errores temprano.
var _ UserRepository = (*MockUserRepository)(nil)

func (m *MockUserRepository) Create(ctx context.Context, user *model.User) (*model.User, error) {
	args := m.Called(ctx, user)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

func (m *MockUserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

func (m *MockUserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

func (m *MockUserRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

func (m *MockUserRepository) List(ctx context.Context) ([]*model.User, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.User), args.Error(1)
}

func (m *MockUserRepository) UpdateLevel(ctx context.Context, id string, level model.UserLevel) error {
	args := m.Called(ctx, id, level)
	return args.Error(0)
}

func (m *MockUserRepository) CountAdmins(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

func (m *MockUserRepository) Deactivate(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
