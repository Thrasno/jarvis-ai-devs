package repository

// MockPromptRepository es el doble de test para PromptRepository.
// Mismo patrón que MockMemoryRepository — embebe mock.Mock de testify.
//
// Upsert devuelve (bool, error):
//   - true  → prompt insertado (nueva fila)
//   - false → sync_id ya existía (DO NOTHING)
//   - error → fallo de base de datos

import (
	"context"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/stretchr/testify/mock"
)

// MockPromptRepository implementa PromptRepository con datos falsos en memoria.
// Úsalo en los tests de SyncService.
type MockPromptRepository struct {
	mock.Mock
}

// Verificación en tiempo de compilación — si falta algún método de la interfaz,
// el compilador dice exactamente cuál falta.
var _ PromptRepository = (*MockPromptRepository)(nil)

// Upsert simula la inserción idempotente de un prompt.
// En el test configuramos con:
//
//	mockRepo.On("Upsert", ctx, prompt).Return(true, nil)
//
// Los dos valores mapean a:
//
//	args.Bool(0) → bool (true = fue INSERT, false = DO NOTHING)
//	args.Error(1) → error
func (m *MockPromptRepository) Upsert(ctx context.Context, p *model.Prompt) (bool, error) {
	args := m.Called(ctx, p)
	return args.Bool(0), args.Error(1)
}
