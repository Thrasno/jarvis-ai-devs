// Package service contiene la lógica de negocio de la aplicación.
//
// La separación handler → service → repository es el patrón más importante
// de esta arquitectura. Cada capa tiene UNA responsabilidad:
//
//   Handler:     hablar HTTP (leer request, escribir response)
//   Service:     ejecutar la lógica de negocio
//   Repository:  hablar con la base de datos
//
// Esta separación permite testear la lógica sin HTTP ni base de datos real.
package service

import (
	"context"
	"errors"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/repository"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// --- Errores del dominio ---

// ErrInvalidCredentials se devuelve cuando email o password son incorrectos.
//
// NOTA DE SEGURIDAD: es deliberadamente vago. No decimos "email no existe" ni
// "password incorrecto" porque eso permitiría a un atacante saber qué emails
// están registrados en el sistema (user enumeration attack).
var ErrInvalidCredentials = errors.New("credenciales inválidas")

// ErrUserInactive se devuelve cuando el usuario existe pero está desactivado.
// El handler lo mapea a HTTP 403 Forbidden (vs 401 para credenciales inválidas).
var ErrUserInactive = errors.New("usuario inactivo")

// --- Interfaz ---

// AuthService define las operaciones de autenticación.
//
// Por qué definimos la interfaz en el mismo paquete que la implementación
// (y no en un paquete separado)?
// Principio de Go: define la interfaz donde SE USA, no donde se implementa.
// Los handlers importarán service.AuthService — lo que importa es que exista aquí.
type AuthService interface {
	// Login verifica credenciales y devuelve un JWT firmado si son válidas.
	// Devuelve ErrInvalidCredentials si email/password no coinciden.
	// Devuelve ErrUserInactive si el usuario está desactivado.
	Login(ctx context.Context, email, password string) (string, error)

	// ValidateToken verifica la firma y expiración del JWT.
	// Devuelve los Claims (payload del token) si es válido.
	// Devuelve error si el token está expirado, malformado, o tiene firma inválida.
	ValidateToken(tokenString string) (*model.Claims, error)
}

// --- Implementación ---

// authService es la implementación concreta. Es privada (minúscula)
// porque los consumidores solo deben conocer la interfaz AuthService,
// no los detalles de implementación.
type authService struct {
	userRepo  repository.UserRepository // inyectado — cualquier implementación sirve
	jwtSecret []byte                    // clave para firmar/verificar JWT
	jwtTTL   time.Duration             // cuánto tiempo es válido un token
}

// NewAuthService crea un nuevo AuthService con sus dependencias inyectadas.
//
// Este patrón se llama "Constructor function" en Go.
// Es equivalente a un constructor PHP:
//   public function __construct(UserRepository $repo, string $jwtSecret) {}
//
// Devuelve la interfaz, no el struct concreto — los consumidores no saben
// qué implementación están usando.
func NewAuthService(userRepo repository.UserRepository, jwtSecret string) AuthService {
	return &authService{
		userRepo:  userRepo,
		jwtSecret: []byte(jwtSecret),
		jwtTTL:   24 * time.Hour, // los tokens duran 24 horas
	}
}

// Login implementa la autenticación completa:
//   1. Buscar usuario por email
//   2. Verificar que el password coincide con el hash bcrypt
//   3. Verificar que el usuario está activo
//   4. Generar y firmar un JWT con los datos del usuario
func (s *authService) Login(ctx context.Context, email, password string) (string, error) {
	// Paso 1: buscar el usuario.
	// Si no existe, el repo devuelve ErrNotFound.
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// Traducimos ErrNotFound a ErrInvalidCredentials (genérico).
			// ¡Nunca devuelvas "usuario no encontrado" al cliente!
			return "", ErrInvalidCredentials
		}
		return "", err // error inesperado de base de datos — lo propagamos
	}

	// Paso 2: verificar que el usuario está activo.
	// Lo hacemos ANTES de verificar el password para evitar timing attacks:
	// si verificamos bcrypt antes, un atacante podría notar que los usuarios
	// inactivos tardan más (bcrypt es lento) → confirma que el email existe.
	if !user.IsActive {
		return "", ErrUserInactive
	}

	// Paso 3: verificar la contraseña.
	// bcrypt.CompareHashAndPassword toma el hash guardado y la contraseña en claro.
	// Si no coinciden, devuelve bcrypt.ErrMismatchedHashAndPassword.
	//
	// ¿Por qué bcrypt y no MD5/SHA256?
	// bcrypt es intencionalmente lento y tiene un "factor de coste" configurable.
	// Esto hace que los ataques de fuerza bruta sean computacionalmente caros.
	// MD5 puede probar millones de contraseñas/segundo; bcrypt, miles.
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", ErrInvalidCredentials // no revelamos que el email SÍ existe
	}

	// Paso 4: generar el JWT.
	return s.generateToken(user)
}

// generateToken crea y firma un JWT para el usuario dado.
//
// Un JWT tiene 3 partes separadas por puntos:
//   header.payload.signature
//
// - header: algoritmo usado (HS256)
// - payload: Claims — los datos que guardamos (ID, username, level, expiración)
// - signature: HMAC-SHA256 del header+payload firmado con jwtSecret
//
// Solo quien tiene jwtSecret puede crear tokens válidos.
// Cualquiera puede LEER el payload (base64, no cifrado) pero no puede FALSIFICARLO.
func (s *authService) generateToken(user *model.User) (string, error) {
	now := time.Now()

	claims := &model.Claims{
		// RegisteredClaims son los campos estándar del estándar JWT (RFC 7519):
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,                               // "sub" — identificador del sujeto
			IssuedAt:  jwt.NewNumericDate(now),               // "iat" — cuándo fue emitido
			ExpiresAt: jwt.NewNumericDate(now.Add(s.jwtTTL)), // "exp" — cuándo expira
		},
		// Claims personalizados — información que necesitamos en cada request
		// para no ir a la BD a buscar el usuario en cada llamada:
		Username: user.Username,
		Level:    user.Level,
	}

	// Creamos el token con el algoritmo HS256 (HMAC + SHA-256).
	// HS256 es simétrico: misma clave para firmar y verificar.
	// Para producción real usaríamos RS256 (asimétrico), pero HS256 es suficiente aquí.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Firmamos el token con nuestro secret y obtenemos el string final.
	return token.SignedString(s.jwtSecret)
}

// ValidateToken verifica y parsea un JWT.
// Si el token es válido, devuelve los Claims para que el handler los use.
func (s *authService) ValidateToken(tokenString string) (*model.Claims, error) {
	claims := &model.Claims{}

	// jwt.ParseWithClaims hace tres cosas en una:
	//   1. Parsea el string JWT en sus 3 partes
	//   2. Verifica la firma usando nuestro secret
	//   3. Verifica que no ha expirado
	//
	// La función keyFunc se llama para obtener la clave de verificación.
	// Recibe el token (ya parseado pero aún no verificado) para que podamos
	// elegir la clave según el algoritmo. Aquí solo aceptamos HS256.
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Verificación de seguridad: aseguramos que el token usa HS256.
		// Sin esto, un atacante podría enviar un token con alg:"none" y eludir la verificación.
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("método de firma inesperado")
		}
		return s.jwtSecret, nil
	})

	if err != nil {
		return nil, err // expirado, malformado, firma inválida, etc.
	}

	if !token.Valid {
		return nil, errors.New("token inválido")
	}

	return claims, nil
}
