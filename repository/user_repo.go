package repository

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// UpdateEmail changes a user's login email. Callers must check EmailExists
// first — this does not enforce uniqueness itself beyond the DB's own
// constraint, so a duplicate will surface as a generic DB error.
func (r *UserRepository) UpdateEmail(ctx context.Context, id, email string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE users SET email = $1, updated_at = NOW() WHERE id = $2::UUID AND deleted_at IS NULL`, email, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// SetCollegeID transfers a user to a different college. The core
// users.college_id update always happens; the cascade to students.college_id
// (the deliberate denormalized copy used for the per-college
// registration-number unique constraint — schema_updates_college_multitenancy_v2.sql)
// is a SEPARATE best-effort statement, not nested in the same transaction —
// nesting it would abort the whole transfer if that column isn't applied
// yet, exactly the class of bug this codebase has hit before.
func (r *UserRepository) SetCollegeID(ctx context.Context, userID, collegeID string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE users SET college_id = $2::UUID, updated_at = NOW() WHERE id = $1::UUID AND deleted_at IS NULL`, userID, collegeID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	if _, err := r.pool.Exec(ctx, `UPDATE students SET college_id = $2::UUID WHERE user_id = $1::UUID`, userID, collegeID); err != nil {
		log.Printf("cascade college_id to student profile for user %s (migration pending?): %v", userID, err)
	}
	return nil
}

// RecordCollegeTransfer writes an audit-trail row for a college transfer.
// Best-effort by convention with every other new-table write introduced this
// stage — a failure here is logged, never propagated, since the transfer
// itself (SetCollegeID) already committed.
func (r *UserRepository) RecordCollegeTransfer(ctx context.Context, userID, oldCollegeID, newCollegeID, movedBy, reason string) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		_, err := r.pool.Exec(ctx, `
			INSERT INTO user_college_history (short_id, user_id, old_college_id, new_college_id, moved_by, reason)
			VALUES ($1, $2::UUID, NULLIF($3,'')::UUID, $4::UUID, $5::UUID, NULLIF($6,''))`,
			shortID, userID, oldCollegeID, newCollegeID, movedBy, reason,
		)
		if err == nil {
			return
		}
		if strings.Contains(err.Error(), "duplicate key") {
			continue
		}
		log.Printf("record college transfer history for user %s (migration pending?): %v", userID, err)
		return
	}
}

func (r *UserRepository) EmailExists(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE LOWER(email) = LOWER($1) AND deleted_at IS NULL)`, email,
	).Scan(&exists)
	return exists, err
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	const q = `
		SELECT id, email, password_hash, first_name, last_name,
		       COALESCE(phone, ''), COALESCE(date_of_birth::TEXT, ''),
		       role, is_active, created_at, updated_at
		FROM users
		WHERE LOWER(email) = LOWER($1) AND is_active = TRUE AND deleted_at IS NULL
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

// GetCollegeID returns a user's college_id (empty string if unset). Kept as
// its own query, separate from FindByEmail/FindByID, so that login and every
// other pre-existing read path keep working unmodified if this column's
// migration (schema_updates_college_v1.sql) hasn't been applied yet — a
// failure here should be treated as "no college" by the caller, never as a
// reason to fail the request it's part of.
func (r *UserRepository) GetCollegeID(ctx context.Context, userID string) (string, error) {
	var collegeID string
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(college_id::TEXT, '') FROM users WHERE id = $1::UUID`, userID).Scan(&collegeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return collegeID, err
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

// FindAll returns all active (non-deleted) users. collegeID scopes the list
// for non-super-admin callers (empty = unscoped).
func (r *UserRepository) FindAll(ctx context.Context, collegeID string) ([]models.User, error) {
	q := `
		SELECT id, email, first_name, last_name,
		       COALESCE(phone, ''), COALESCE(date_of_birth::TEXT, ''),
		       role, is_active, created_at, updated_at, COALESCE(college_id::TEXT, '')
		FROM users
		WHERE deleted_at IS NULL`
	args := []interface{}{}
	if collegeID != "" {
		q += ` AND college_id = $1::UUID`
		args = append(args, collegeID)
	}
	q += ` ORDER BY created_at DESC`
	return r.scanUsers(ctx, q, args...)
}

// FindDeleted returns all soft-deleted users.
func (r *UserRepository) FindDeleted(ctx context.Context) ([]models.User, error) {
	const q = `
		SELECT id, email, first_name, last_name,
		       COALESCE(phone, ''), COALESCE(date_of_birth::TEXT, ''),
		       role, is_active, created_at, updated_at, COALESCE(college_id::TEXT, '')
		FROM users
		WHERE deleted_at IS NOT NULL
		ORDER BY deleted_at DESC`

	return r.scanUsers(ctx, q)
}

// FindByRole returns all non-deleted users with the given role. collegeID
// scopes the list for non-super-admin callers (empty = unscoped).
func (r *UserRepository) FindByRole(ctx context.Context, role models.Role, collegeID string) ([]models.User, error) {
	q := `
		SELECT id, email, first_name, last_name,
		       COALESCE(phone, ''), COALESCE(date_of_birth::TEXT, ''),
		       role, is_active, created_at, updated_at, COALESCE(college_id::TEXT, '')
		FROM users
		WHERE role = $1 AND deleted_at IS NULL`
	args := []interface{}{role}
	if collegeID != "" {
		q += ` AND college_id = $2::UUID`
		args = append(args, collegeID)
	}
	q += ` ORDER BY first_name, last_name`
	return r.scanUsers(ctx, q, args...)
}

// SearchUsers returns non-deleted users matching the query against name,
// email, phone, or ID. collegeID scopes the results for non-super-admin
// callers (empty = unscoped).
func (r *UserRepository) SearchUsers(ctx context.Context, query, collegeID string) ([]models.User, error) {
	q := `
		SELECT id, email, first_name, last_name,
		       COALESCE(phone, ''), COALESCE(date_of_birth::TEXT, ''),
		       role, is_active, created_at, updated_at, COALESCE(college_id::TEXT, '')
		FROM users
		WHERE deleted_at IS NULL
		  AND (
		        id::TEXT ILIKE $1
		     OR email ILIKE $1
		     OR phone ILIKE $1
		     OR first_name ILIKE $1
		     OR last_name ILIKE $1
		     OR CONCAT(first_name, ' ', last_name) ILIKE $1
		  )`
	args := []interface{}{"%" + query + "%"}
	if collegeID != "" {
		q += ` AND college_id = $2::UUID`
		args = append(args, collegeID)
	}
	q += ` ORDER BY created_at DESC`
	return r.scanUsers(ctx, q, args...)
}

// FindStudentsForMentor returns students enrolled in any batch the given
// mentor manages (batch_manager_id or additional_manager_id) — used to scope
// a mentor's student list down to their own batches instead of every
// student on the platform.
func (r *UserRepository) FindStudentsForMentor(ctx context.Context, mentorID string) ([]models.User, error) {
	const q = `
		SELECT DISTINCT u.id, u.email, u.first_name, u.last_name,
		       COALESCE(u.phone, ''), COALESCE(u.date_of_birth::TEXT, ''),
		       u.role, u.is_active, u.created_at, u.updated_at, COALESCE(u.college_id::TEXT, '')
		FROM users u
		JOIN batch_students bs ON bs.user_id = u.id
		JOIN batches b ON b.id = bs.batch_id AND b.deleted_at IS NULL
		WHERE u.deleted_at IS NULL
		  AND (b.batch_manager_id = $1 OR b.additional_manager_id = $1)
		ORDER BY u.created_at DESC`
	return r.scanUsers(ctx, q, mentorID)
}

// SearchStudentsForMentor mirrors SearchUsers but scoped to students in a
// batch the given mentor manages, so a mentor can't look up a student
// outside their own batches just by knowing a name/email/ID to search for.
func (r *UserRepository) SearchStudentsForMentor(ctx context.Context, mentorID, query string) ([]models.User, error) {
	const q = `
		SELECT DISTINCT u.id, u.email, u.first_name, u.last_name,
		       COALESCE(u.phone, ''), COALESCE(u.date_of_birth::TEXT, ''),
		       u.role, u.is_active, u.created_at, u.updated_at, COALESCE(u.college_id::TEXT, '')
		FROM users u
		JOIN batch_students bs ON bs.user_id = u.id
		JOIN batches b ON b.id = bs.batch_id AND b.deleted_at IS NULL
		WHERE u.deleted_at IS NULL
		  AND (b.batch_manager_id = $1 OR b.additional_manager_id = $1)
		  AND (
		        u.id::TEXT ILIKE $2
		     OR u.email ILIKE $2
		     OR u.phone ILIKE $2
		     OR u.first_name ILIKE $2
		     OR u.last_name ILIKE $2
		     OR CONCAT(u.first_name, ' ', u.last_name) ILIKE $2
		  )
		ORDER BY u.created_at DESC`
	return r.scanUsers(ctx, q, mentorID, "%"+query+"%")
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
			&u.Phone, &u.DateOfBirth, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt, &u.CollegeID,
		); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// Register inserts a user row and a student profile row in a single
// transaction. All new registrations default to the student role.
// collegeID must never be empty — callers resolve it beforehand (self-signup
// always uses CollegeRepository.DefaultCollegeID; admin-provisioned students
// resolve either an explicit college or also default to it) so that no
// newly created user is ever left with a NULL college_id.
//
// registrationNo is optional (nil when CollegeRepository.NextRegistrationNo
// reports the migration adding it hasn't been applied yet) and is written
// via a separate best-effort statement AFTER the core transaction commits —
// deliberately not nested inside that transaction, since a failed statement
// there would abort the whole registration rather than degrade gracefully.
// The same registrationNo also seeds the student's roll number
// (students.enrollment_no), formatted as "<registration year><3-digit
// sequence>" (e.g. 2026001) — enrollment_no stays editable afterward via
// StudentRegistrationRepository.Upsert, e.g. to apply an explicit roll
// number from a bulk import row.
func (r *UserRepository) Register(ctx context.Context, user models.User, collegeID string, registrationNo *int) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, first_name, last_name, phone, date_of_birth, role, college_id)
		VALUES ($1, $2, $3, $4, NULLIF($5,''), NULLIF($6,'')::DATE, 'student', $7::UUID)
		RETURNING id`,
		user.Email, user.PasswordHash, user.FirstName, user.LastName,
		user.Phone, user.DateOfBirth, collegeID,
	).Scan(&userID)
	if err != nil {
		return "", fmt.Errorf("insert user: %w", err)
	}

	if _, err = tx.Exec(ctx, `INSERT INTO students (user_id) VALUES ($1)`, userID); err != nil {
		return "", fmt.Errorf("insert student profile: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}

	if registrationNo != nil {
		enrollmentNo := fmt.Sprintf("%d%03d", time.Now().Year(), *registrationNo)
		if _, err := r.pool.Exec(ctx, `UPDATE students SET registration_no = $2, college_id = $3::UUID, enrollment_no = $4 WHERE user_id = $1`,
			userID, *registrationNo, collegeID, enrollmentNo,
		); err != nil {
			log.Printf("set registration_no/enrollment_no/college_id for new student %s (migration pending?): %v", userID, err)
		}
	}

	return userID, nil
}

// CreateStaffUser inserts a user row with the given role plus an empty
// role-specific profile row, in a single transaction. Only roles present in
// profileTable are supported (mentor, employee, team_lead) — student accounts
// go through Register, and there is no backing table for super_admin yet.
// collegeID must never be empty — see the identical note on Register.
// CreateStaffUser inserts a user row with the given role plus an empty
// role-specific profile row, in a single transaction — mentor/employee/
// team_lead each get a dedicated profile row (profileTable). Roles with no
// entry in profileTable (college_admin, college_staff) skip that insert
// entirely rather than erroring — there's no dedicated schema for them yet,
// they're just users rows scoped by college_id.
func (r *UserRepository) CreateStaffUser(ctx context.Context, user models.User, role models.Role, collegeID string) (string, error) {
	table, hasProfileTable := profileTable[role]

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, first_name, last_name, phone, role, college_id)
		VALUES ($1, $2, $3, $4, NULLIF($5,''), $6, $7::UUID)
		RETURNING id`,
		user.Email, user.PasswordHash, user.FirstName, user.LastName, user.Phone, role, collegeID,
	).Scan(&userID)
	if err != nil {
		return "", fmt.Errorf("insert user: %w", err)
	}

	if hasProfileTable {
		if _, err = tx.Exec(ctx, fmt.Sprintf(`INSERT INTO %s (user_id) VALUES ($1)`, table), userID); err != nil {
			return "", fmt.Errorf("insert %s profile: %w", table, err)
		}
	}

	return userID, tx.Commit(ctx)
}

// profileTable maps a role to its dedicated profile table.
var profileTable = map[models.Role]string{
	models.RoleStudent:  "students",
	models.RoleMentor:   "mentors",
	models.RoleEmployee: "employees",
	models.RoleTeamLead: "team_leads",
}

// ChangeUserRole updates a user's role and swaps their role-specific profile row atomically.
// The old profile row is deleted and a new empty one is created in the target table — for a
// student, that DELETE takes registration_no/enrollment_no (their roll number) with it, since
// roll numbers only ever make sense for the student role. Moving a user INTO the student role
// gets them a freshly assigned roll number, same as a new registration (best-effort: skipped,
// not fatal, if the registration-numbering migration hasn't been applied yet).
func (r *UserRepository) ChangeUserRole(ctx context.Context, userID string, newRole models.Role) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Fetch current role
	var currentRole models.Role
	var collegeID *string
	err = tx.QueryRow(ctx,
		`SELECT role, college_id::TEXT FROM users WHERE id = $1 AND deleted_at IS NULL`, userID,
	).Scan(&currentRole, &collegeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgx.ErrNoRows
	}
	if err != nil {
		return fmt.Errorf("fetch role: %w", err)
	}

	if currentRole == newRole {
		return nil
	}

	oldTable, ok := profileTable[currentRole]
	if !ok {
		return fmt.Errorf("no profile table for current role %q", currentRole)
	}
	newTable, ok := profileTable[newRole]
	if !ok {
		return fmt.Errorf("no profile table for new role %q", newRole)
	}

	if _, err = tx.Exec(ctx,
		fmt.Sprintf(`DELETE FROM %s WHERE user_id = $1`, oldTable), userID,
	); err != nil {
		return fmt.Errorf("delete old profile: %w", err)
	}

	if _, err = tx.Exec(ctx,
		fmt.Sprintf(`INSERT INTO %s (user_id) VALUES ($1)`, newTable), userID,
	); err != nil {
		return fmt.Errorf("insert new profile: %w", err)
	}

	if _, err = tx.Exec(ctx,
		`UPDATE users SET role = $1 WHERE id = $2`, newRole, userID,
	); err != nil {
		return fmt.Errorf("update role: %w", err)
	}

	if newRole == models.RoleStudent && collegeID != nil {
		var n int
		if err := tx.QueryRow(ctx, `
			UPDATE colleges SET next_registration_no = next_registration_no + 1
			WHERE id = $1::UUID AND deleted_at IS NULL
			RETURNING next_registration_no - 1`,
			*collegeID,
		).Scan(&n); err == nil {
			enrollmentNo := fmt.Sprintf("%d%03d", time.Now().Year(), n)
			if _, err := tx.Exec(ctx,
				`UPDATE students SET registration_no = $2, college_id = $3::UUID, enrollment_no = $4 WHERE user_id = $1`,
				userID, n, *collegeID, enrollmentNo,
			); err != nil {
				return fmt.Errorf("set roll number for new student %s: %w", userID, err)
			}
		}
	}

	return tx.Commit(ctx)
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
	if in.IsActive != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_active = $%d", i))
		args = append(args, *in.IsActive)
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

// FindPasswordHashByID returns the bcrypt hash stored for an active user, for
// verifying the "old password" on a change-password request.
func (r *UserRepository) FindPasswordHashByID(ctx context.Context, id string) (string, error) {
	var hash string
	err := r.pool.QueryRow(ctx,
		`SELECT password_hash FROM users WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", pgx.ErrNoRows
	}
	if err != nil {
		return "", err
	}
	return hash, nil
}

// UpdatePassword overwrites the stored bcrypt hash for a user.
func (r *UserRepository) UpdatePassword(ctx context.Context, id string, newHash string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE users SET password_hash = $1 WHERE id = $2 AND deleted_at IS NULL`, newHash, id,
	)
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
