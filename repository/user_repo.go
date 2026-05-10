package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) EmailExists(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE email = $1 AND deleted_at IS NULL)`, email,
	).Scan(&exists)
	return exists, err
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	const q = `
		SELECT id, email, password_hash, first_name, last_name,
		       COALESCE(phone, ''), COALESCE(date_of_birth::TEXT, ''),
		       role, is_active, created_at, updated_at
		FROM users
		WHERE email = $1 AND is_active = TRUE AND deleted_at IS NULL
		LIMIT 1`

	var u models.User
	err := r.pool.QueryRow(ctx, q, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.FirstName, &u.LastName,
		&u.Phone, &u.DateOfBirth, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*models.User, error) {
	const q = `
		SELECT id, email, first_name, last_name,
		       COALESCE(phone, ''), COALESCE(date_of_birth::TEXT, ''),
		       role, is_active, created_at, updated_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
		LIMIT 1`

	var u models.User
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&u.ID, &u.Email, &u.FirstName, &u.LastName,
		&u.Phone, &u.DateOfBirth, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// FindAll returns all active (non-deleted) users.
func (r *UserRepository) FindAll(ctx context.Context) ([]models.User, error) {
	const q = `
		SELECT id, email, first_name, last_name,
		       COALESCE(phone, ''), COALESCE(date_of_birth::TEXT, ''),
		       role, is_active, created_at, updated_at
		FROM users
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC`

	return r.scanUsers(ctx, q)
}

// FindDeleted returns all soft-deleted users.
func (r *UserRepository) FindDeleted(ctx context.Context) ([]models.User, error) {
	const q = `
		SELECT id, email, first_name, last_name,
		       COALESCE(phone, ''), COALESCE(date_of_birth::TEXT, ''),
		       role, is_active, created_at, updated_at
		FROM users
		WHERE deleted_at IS NOT NULL
		ORDER BY deleted_at DESC`

	return r.scanUsers(ctx, q)
}

// FindByRole returns all non-deleted users with the given role.
func (r *UserRepository) FindByRole(ctx context.Context, role models.Role) ([]models.User, error) {
	const q = `
		SELECT id, email, first_name, last_name,
		       COALESCE(phone, ''), COALESCE(date_of_birth::TEXT, ''),
		       role, is_active, created_at, updated_at
		FROM users
		WHERE role = $1 AND deleted_at IS NULL
		ORDER BY first_name, last_name`
	return r.scanUsers(ctx, q, role)
}

// SearchUsers returns non-deleted users matching the query against name, email, phone, or ID.
func (r *UserRepository) SearchUsers(ctx context.Context, query string) ([]models.User, error) {
	const q = `
		SELECT id, email, first_name, last_name,
		       COALESCE(phone, ''), COALESCE(date_of_birth::TEXT, ''),
		       role, is_active, created_at, updated_at
		FROM users
		WHERE deleted_at IS NULL
		  AND (
		        id::TEXT ILIKE $1
		     OR email ILIKE $1
		     OR phone ILIKE $1
		     OR first_name ILIKE $1
		     OR last_name ILIKE $1
		     OR CONCAT(first_name, ' ', last_name) ILIKE $1
		  )
		ORDER BY created_at DESC`
	return r.scanUsers(ctx, q, "%"+query+"%")
}

func (r *UserRepository) scanUsers(ctx context.Context, q string, args ...interface{}) ([]models.User, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(
			&u.ID, &u.Email, &u.FirstName, &u.LastName,
			&u.Phone, &u.DateOfBirth, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// Register inserts a user row and a default profile row for the role in a single transaction.
func (r *UserRepository) Register(ctx context.Context, user models.User) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, first_name, last_name, phone, date_of_birth, role)
		VALUES ($1, $2, $3, $4, NULLIF($5,''), NULLIF($6,'')::DATE, $7)
		RETURNING id`,
		user.Email, user.PasswordHash, user.FirstName, user.LastName,
		user.Phone, user.DateOfBirth, user.Role,
	).Scan(&userID)
	if err != nil {
		return "", fmt.Errorf("insert user: %w", err)
	}

	switch user.Role {
	case models.RoleStudent:
		_, err = tx.Exec(ctx, `INSERT INTO students (user_id) VALUES ($1)`, userID)
	case models.RoleMentor:
		_, err = tx.Exec(ctx, `INSERT INTO mentors (user_id) VALUES ($1)`, userID)
	case models.RoleEmployee:
		_, err = tx.Exec(ctx, `INSERT INTO employees (user_id) VALUES ($1)`, userID)
	case models.RoleTeamLead:
		_, err = tx.Exec(ctx, `INSERT INTO team_leads (user_id) VALUES ($1)`, userID)
	case models.RoleSuperAdmin:
		_, err = tx.Exec(ctx, `INSERT INTO super_admins (user_id) VALUES ($1)`, userID)
	}

	if err != nil {
		return "", fmt.Errorf("insert profile: %w", err)
	}

	return userID, tx.Commit(ctx)
}

// UpdateUser applies a partial update — only non-nil fields in the input are changed.
func (r *UserRepository) UpdateUser(ctx context.Context, id string, in models.UpdateUserInput) error {
	args := []interface{}{id}
	setClauses := []string{}
	i := 2

	if in.FirstName != nil {
		setClauses = append(setClauses, fmt.Sprintf("first_name = $%d", i))
		args = append(args, *in.FirstName)
		i++
	}
	if in.LastName != nil {
		setClauses = append(setClauses, fmt.Sprintf("last_name = $%d", i))
		args = append(args, *in.LastName)
		i++
	}
	if in.Phone != nil {
		setClauses = append(setClauses, fmt.Sprintf("phone = NULLIF($%d,'')", i))
		args = append(args, *in.Phone)
		i++
	}
	if in.DateOfBirth != nil {
		setClauses = append(setClauses, fmt.Sprintf("date_of_birth = NULLIF($%d,'')::DATE", i))
		args = append(args, *in.DateOfBirth)
		i++
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}

	q := fmt.Sprintf(
		"UPDATE users SET %s WHERE id = $1 AND deleted_at IS NULL",
		strings.Join(setClauses, ", "),
	)
	result, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// SoftDeleteUser sets deleted_at without removing the row.
func (r *UserRepository) SoftDeleteUser(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE users SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, id,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
