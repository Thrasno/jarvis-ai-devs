package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// postgresUserRepository es la implementación de UserRepository sobre PostgreSQL.
//
// Usamos pgxpool.Pool para todas las queries — el pool gestiona automáticamente
// la concurrencia de conexiones. Nunca guardamos una conexión individual (*pgx.Conn)
// como campo del struct, porque eso impediría la reutilización del pool.
type postgresUserRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresUserRepository crea la implementación real de UserRepository.
// El caller (main.go) pasa el pool; el repositorio solo lo usa.
func NewPostgresUserRepository(pool *pgxpool.Pool) UserRepository {
	return &postgresUserRepository{pool: pool}
}

func (r *postgresUserRepository) Create(ctx context.Context, user *model.User) (*model.User, error) {
	const q = `
		INSERT INTO users (username, email, password, level, is_active)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`

	row := r.pool.QueryRow(ctx, q,
		user.Username, user.Email, user.Password, user.Level, user.IsActive)

	err := row.Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, wrapPgError(err, "Create user")
	}
	return user, nil
}

func (r *postgresUserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	const q = `SELECT id, username, email, password, level, is_active, created_at, updated_at
	           FROM users WHERE id = $1`
	return r.scanUser(ctx, q, id)
}

func (r *postgresUserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	const q = `SELECT id, username, email, password, level, is_active, created_at, updated_at
	           FROM users WHERE email = $1`
	return r.scanUser(ctx, q, email)
}

func (r *postgresUserRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	const q = `SELECT id, username, email, password, level, is_active, created_at, updated_at
	           FROM users WHERE username = $1`
	return r.scanUser(ctx, q, username)
}

func (r *postgresUserRepository) List(ctx context.Context) ([]*model.User, error) {
	const q = `SELECT id, username, email, password, level, is_active, created_at, updated_at
	           FROM users ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, wrapPgError(err, "List users")
	}
	defer rows.Close()

	var users []*model.User
	for rows.Next() {
		u := &model.User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Password,
			&u.Level, &u.IsActive, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, wrapPgError(err, "scan user row")
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *postgresUserRepository) UpdateLevel(ctx context.Context, id string, level model.UserLevel) error {
	const q = `UPDATE users SET level = $1, updated_at = $2 WHERE id = $3`
	_, err := r.pool.Exec(ctx, q, level, time.Now(), id)
	return wrapPgError(err, "UpdateLevel")
}

func (r *postgresUserRepository) CountAdmins(ctx context.Context) (int, error) {
	const q = `SELECT COUNT(*) FROM users WHERE level = 'admin' AND is_active = true`
	var count int
	err := r.pool.QueryRow(ctx, q).Scan(&count)
	return count, wrapPgError(err, "CountAdmins")
}

func (r *postgresUserRepository) Deactivate(ctx context.Context, id string) error {
	const q = `UPDATE users SET is_active = false, updated_at = $1 WHERE id = $2`
	_, err := r.pool.Exec(ctx, q, time.Now(), id)
	return wrapPgError(err, "Deactivate")
}

// scanUser ejecuta una query que devuelve un único usuario y escanea el resultado.
// Centraliza el scan para no repetirlo en GetByID, GetByEmail, GetByUsername.
func (r *postgresUserRepository) scanUser(ctx context.Context, query string, arg interface{}) (*model.User, error) {
	u := &model.User{}
	err := r.pool.QueryRow(ctx, query, arg).Scan(
		&u.ID, &u.Username, &u.Email, &u.Password,
		&u.Level, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, wrapPgError(err, "scanUser")
	}
	return u, nil
}
