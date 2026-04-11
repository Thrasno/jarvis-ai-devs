// Package service_test es el paquete de tests para los services.
//
// Usamos el sufijo _test (caja negra) igual que en config_test:
// testemos la API pública del service, no sus detalles internos.
// Si mañana refactorizamos la implementación, los tests no cambian.
package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// jwtSecret es la clave que usamos en los tests.
// Mínimo 32 caracteres — igual que en producción.
const testJWTSecret = "test-secret-key-for-jwt-32chars!!"

// newTestAuthService es un helper que crea el AuthService con un mock limpio.
// Devuelve tanto el service como el mock para que cada test pueda configurar
// las expectativas del mock de forma independiente.
//
// En Go, los helpers de test suelen empezar con "new" o "make" y
// aceptar *testing.T para poder llamar t.Fatal() si la configuración falla.
func newTestAuthService(t *testing.T) (service.AuthService, *repository.MockUserRepository) {
	t.Helper() // marca esta función como helper para que los errores muestren la línea del test, no de aquí
	mockRepo := &repository.MockUserRepository{}
	svc := service.NewAuthService(mockRepo, testJWTSecret)
	return svc, mockRepo
}

// hashPassword genera un bcrypt hash para usar en los tests.
// bcrypt.MinCost es el mínimo factor de coste — suficiente para tests, más rápido.
func hashPassword(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	return string(hash)
}

// --- Tests de Login ---

// TestLogin_Success verifica el camino feliz: credenciales válidas → JWT devuelto.
func TestLogin_Success(t *testing.T) {
	svc, mockRepo := newTestAuthService(t)
	ctx := context.Background()

	// Preparamos el usuario que devolverá el mock cuando se llame GetByEmail.
	// La contraseña está hasheada con bcrypt — igual que en producción.
	activeUser := &model.User{
		ID:       "user-id-123",
		Username: "andres",
		Email:    "andres@test.com",
		Password: hashPassword(t, "secret123"),
		Level:    model.LevelAdmin,
		IsActive: true,
	}

	// Configuramos el mock: "cuando alguien llame GetByEmail con este email,
	// devuelve este usuario sin error".
	// mock.Anything es un matcher que acepta cualquier valor — lo usamos para ctx
	// porque no nos importa el context específico, solo que se llamó con el email correcto.
	mockRepo.On("GetByEmail", ctx, "andres@test.com").Return(activeUser, nil)

	token, err := svc.Login(ctx, "andres@test.com", "secret123")

	// require.NoError falla el test INMEDIATAMENTE si hay error.
	// Usar require (no assert) cuando el resto del test no tiene sentido si esto falla.
	require.NoError(t, err)

	// assert.NotEmpty verifica que el token no está vacío, sin fallar inmediatamente.
	// Los asserts se acumulan — al final del test ves todos los fallos juntos.
	assert.NotEmpty(t, token)

	// Verificamos que el mock fue llamado exactamente como esperábamos.
	// Si GetByEmail se llamó con argumentos diferentes, esto falla.
	mockRepo.AssertExpectations(t)
}

// TestLogin_UserNotFound verifica que un email inexistente devuelve un error genérico.
//
// CRÍTICO para seguridad: NO revelar si el email existe en el sistema.
// Si devolvemos "usuario no encontrado" vs "contraseña incorrecta", un atacante
// puede enumerar qué emails están registrados (user enumeration attack).
// En su lugar, siempre devolvemos el mismo error: ErrInvalidCredentials.
func TestLogin_UserNotFound(t *testing.T) {
	svc, mockRepo := newTestAuthService(t)
	ctx := context.Background()

	// El repo devuelve ErrNotFound — simula que el email no existe en la BD.
	mockRepo.On("GetByEmail", ctx, "noexiste@test.com").Return(nil, repository.ErrNotFound)

	token, err := svc.Login(ctx, "noexiste@test.com", "cualquierpassword")

	assert.Empty(t, token)
	// El service debe traducir ErrNotFound a ErrInvalidCredentials.
	// El handler verá ErrInvalidCredentials y devolverá HTTP 401 con mensaje genérico.
	assert.ErrorIs(t, err, service.ErrInvalidCredentials)
	mockRepo.AssertExpectations(t)
}

// TestLogin_WrongPassword verifica que una contraseña incorrecta devuelve el mismo error genérico.
func TestLogin_WrongPassword(t *testing.T) {
	svc, mockRepo := newTestAuthService(t)
	ctx := context.Background()

	activeUser := &model.User{
		ID:       "user-id-123",
		Email:    "andres@test.com",
		Password: hashPassword(t, "password_correcto"),
		IsActive: true,
	}

	mockRepo.On("GetByEmail", ctx, "andres@test.com").Return(activeUser, nil)

	// Intentamos con una contraseña diferente
	token, err := svc.Login(ctx, "andres@test.com", "password_incorrecto")

	assert.Empty(t, token)
	// Mismo error que "usuario no encontrado" — no revelamos la causa específica
	assert.ErrorIs(t, err, service.ErrInvalidCredentials)
	mockRepo.AssertExpectations(t)
}

// TestLogin_InactiveUser verifica que un usuario desactivado no puede hacer login.
func TestLogin_InactiveUser(t *testing.T) {
	svc, mockRepo := newTestAuthService(t)
	ctx := context.Background()

	inactiveUser := &model.User{
		ID:       "user-id-123",
		Email:    "andres@test.com",
		Password: hashPassword(t, "secret123"),
		IsActive: false, // ← usuario desactivado
	}

	mockRepo.On("GetByEmail", ctx, "andres@test.com").Return(inactiveUser, nil)

	token, err := svc.Login(ctx, "andres@test.com", "secret123")

	assert.Empty(t, token)
	assert.ErrorIs(t, err, service.ErrUserInactive)
	mockRepo.AssertExpectations(t)
}

// --- Tests de ValidateToken ---

// TestValidateToken_Success verifica que un token válido devuelve los Claims correctos.
func TestValidateToken_Success(t *testing.T) {
	svc, mockRepo := newTestAuthService(t)
	ctx := context.Background()

	// Paso 1: generar un token real mediante Login.
	// Esto testea Login + ValidateToken juntos de forma realista.
	activeUser := &model.User{
		ID:       "user-id-123",
		Username: "andres",
		Level:    model.LevelAdmin,
		IsActive: true,
		Password: hashPassword(t, "secret123"),
	}
	mockRepo.On("GetByEmail", ctx, "andres@test.com").Return(activeUser, nil)

	token, err := svc.Login(ctx, "andres@test.com", "secret123")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	// Paso 2: validar el token y verificar que los Claims son correctos.
	claims, err := svc.ValidateToken(token)

	require.NoError(t, err)
	require.NotNil(t, claims)
	assert.Equal(t, "user-id-123", claims.Subject) // Subject es el estándar JWT para el ID del usuario
	assert.Equal(t, "andres", claims.Username)
	assert.Equal(t, model.LevelAdmin, claims.Level)

	// El token debe expirar en el futuro (no ya).
	assert.True(t, claims.ExpiresAt.Time.After(time.Now()))
}

// TestValidateToken_InvalidSignature verifica que un token con firma incorrecta es rechazado.
// Simula que alguien intenta forjar un token cambiando el secret.
func TestValidateToken_InvalidSignature(t *testing.T) {
	// Creamos un service con un secret diferente para generar el token
	differentSecretSvc := service.NewAuthService(nil, "different-secret-key-32chars!!")
	ctx := context.Background()

	// Hmm — necesitamos un user para Login. Usamos otro mock temporal.
	// En realidad el test más limpio es manipular el token directamente.
	// Usamos un token JWT fabricado con secret diferente.
	// Para simplificar, generamos el token con un service diferente.
	_ = ctx // silencia el compilador

	// Generamos token con el service principal (testJWTSecret)
	svc, mockRepo := newTestAuthService(t)
	activeUser := &model.User{
		ID: "u1", Username: "x", Level: model.LevelMember,
		IsActive: true, Password: hashPassword(t, "pw"),
	}
	mockRepo.On("GetByEmail", context.Background(), "x@test.com").Return(activeUser, nil)
	token, err := svc.Login(context.Background(), "x@test.com", "pw")
	require.NoError(t, err)

	// Validamos ese token con el service que tiene un SECRET DIFERENTE.
	// Debe rechazarlo.
	claims, err := differentSecretSvc.ValidateToken(token)

	assert.Error(t, err)
	assert.Nil(t, claims)
}

// TestValidateToken_MalformedToken verifica que un string aleatorio es rechazado.
func TestValidateToken_MalformedToken(t *testing.T) {
	svc, _ := newTestAuthService(t)

	claims, err := svc.ValidateToken("esto-no-es-un-jwt")

	assert.Error(t, err)
	assert.Nil(t, claims)
}
