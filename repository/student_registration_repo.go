package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type StudentRegistrationRepository struct {
	pool *pgxpool.Pool
}

func NewStudentRegistrationRepository(pool *pgxpool.Pool) *StudentRegistrationRepository {
	return &StudentRegistrationRepository{pool: pool}
}

// GetByUserID returns the student's registration details, or a zero-value
// (empty) struct — never nil, never an error — if they haven't saved any yet.
func (r *StudentRegistrationRepository) GetByUserID(ctx context.Context, userID string) (*models.StudentRegistrationDetails, error) {
	var d models.StudentRegistrationDetails
	err := r.pool.QueryRow(ctx, `
		SELECT u.id, COALESCE(s.status::TEXT, ''), COALESCE(s.enrollment_no, ''),
		       COALESCE(srd.gender, ''), COALESCE(srd.alternate_contact, ''), COALESCE(srd.student_source, ''),
		       COALESCE(srd.religion, ''), COALESCE(srd.standard, ''), COALESCE(srd.occupation, ''), COALESCE(srd.timezone, ''),
		       COALESCE(srd.parent_name, ''), COALESCE(srd.parent_contact, ''), COALESCE(srd.parent_email, ''), COALESCE(srd.area, ''),
		       COALESCE(srd.school_college_name, ''), COALESCE(srd.residential_address, ''), COALESCE(srd.permanent_address, ''),
		       COALESCE(srd.city, ''), COALESCE(srd.state, ''), COALESCE(srd.pincode, ''),
		       COALESCE(srd.opt_whatsapp, TRUE), COALESCE(srd.opt_email, TRUE), COALESCE(srd.opt_sms, TRUE), COALESCE(srd.opt_push, TRUE),
		       COALESCE(srd.updated_at, u.updated_at)
		FROM users u
		LEFT JOIN students s ON s.user_id = u.id
		LEFT JOIN student_registration_details srd ON srd.user_id = u.id
		WHERE u.id = $1::UUID`,
		userID,
	).Scan(
		&d.UserID, &d.Status, &d.EnrollmentNo,
		&d.Gender, &d.AlternateContact, &d.StudentSource,
		&d.Religion, &d.Standard, &d.Occupation, &d.Timezone,
		&d.ParentName, &d.ParentContact, &d.ParentEmail, &d.Area,
		&d.SchoolCollegeName, &d.ResidentialAddress, &d.PermanentAddress,
		&d.City, &d.State, &d.Pincode,
		&d.OptWhatsapp, &d.OptEmail, &d.OptSMS, &d.OptPush,
		&d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

// Upsert creates or updates the registration-details row for userID with
// whatever fields are non-nil in the input.
func (r *StudentRegistrationRepository) Upsert(ctx context.Context, userID string, in models.UpdateStudentRegistrationInput) error {
	current, err := r.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}
	d := models.StudentRegistrationDetails{OptWhatsapp: true, OptEmail: true, OptSMS: true, OptPush: true}
	if current != nil {
		d = *current
	}
	if in.Gender != nil {
		d.Gender = *in.Gender
	}
	if in.AlternateContact != nil {
		d.AlternateContact = *in.AlternateContact
	}
	if in.StudentSource != nil {
		d.StudentSource = *in.StudentSource
	}
	if in.Religion != nil {
		d.Religion = *in.Religion
	}
	if in.Standard != nil {
		d.Standard = *in.Standard
	}
	if in.Occupation != nil {
		d.Occupation = *in.Occupation
	}
	if in.Timezone != nil {
		d.Timezone = *in.Timezone
	}
	if in.ParentName != nil {
		d.ParentName = *in.ParentName
	}
	if in.ParentContact != nil {
		d.ParentContact = *in.ParentContact
	}
	if in.ParentEmail != nil {
		d.ParentEmail = *in.ParentEmail
	}
	if in.Area != nil {
		d.Area = *in.Area
	}
	if in.SchoolCollegeName != nil {
		d.SchoolCollegeName = *in.SchoolCollegeName
	}
	if in.ResidentialAddress != nil {
		d.ResidentialAddress = *in.ResidentialAddress
	}
	if in.PermanentAddress != nil {
		d.PermanentAddress = *in.PermanentAddress
	}
	if in.City != nil {
		d.City = *in.City
	}
	if in.State != nil {
		d.State = *in.State
	}
	if in.Pincode != nil {
		d.Pincode = *in.Pincode
	}
	if in.OptWhatsapp != nil {
		d.OptWhatsapp = *in.OptWhatsapp
	}
	if in.OptEmail != nil {
		d.OptEmail = *in.OptEmail
	}
	if in.OptSMS != nil {
		d.OptSMS = *in.OptSMS
	}
	if in.OptPush != nil {
		d.OptPush = *in.OptPush
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO student_registration_details (
			user_id, gender, alternate_contact, student_source, religion, standard, occupation, timezone,
			parent_name, parent_contact, parent_email, area, school_college_name, residential_address, permanent_address,
			city, state, pincode, opt_whatsapp, opt_email, opt_sms, opt_push, updated_at
		) VALUES (
			$1::UUID, NULLIF($2,''), NULLIF($3,''), NULLIF($4,''), NULLIF($5,''), NULLIF($6,''), NULLIF($7,''), NULLIF($8,''),
			NULLIF($9,''), NULLIF($10,''), NULLIF($11,''), NULLIF($12,''), NULLIF($13,''), NULLIF($14,''), NULLIF($15,''),
			NULLIF($16,''), NULLIF($17,''), NULLIF($18,''), $19, $20, $21, $22, NOW()
		)
		ON CONFLICT (user_id) DO UPDATE SET
			gender = EXCLUDED.gender, alternate_contact = EXCLUDED.alternate_contact, student_source = EXCLUDED.student_source,
			religion = EXCLUDED.religion, standard = EXCLUDED.standard, occupation = EXCLUDED.occupation, timezone = EXCLUDED.timezone,
			parent_name = EXCLUDED.parent_name, parent_contact = EXCLUDED.parent_contact, parent_email = EXCLUDED.parent_email,
			area = EXCLUDED.area, school_college_name = EXCLUDED.school_college_name,
			residential_address = EXCLUDED.residential_address, permanent_address = EXCLUDED.permanent_address,
			city = EXCLUDED.city, state = EXCLUDED.state, pincode = EXCLUDED.pincode,
			opt_whatsapp = EXCLUDED.opt_whatsapp, opt_email = EXCLUDED.opt_email, opt_sms = EXCLUDED.opt_sms, opt_push = EXCLUDED.opt_push,
			updated_at = NOW()`,
		userID, d.Gender, d.AlternateContact, d.StudentSource, d.Religion, d.Standard, d.Occupation, d.Timezone,
		d.ParentName, d.ParentContact, d.ParentEmail, d.Area, d.SchoolCollegeName, d.ResidentialAddress, d.PermanentAddress,
		d.City, d.State, d.Pincode, d.OptWhatsapp, d.OptEmail, d.OptSMS, d.OptPush,
	)
	if err != nil {
		return err
	}

	// enrollment_no lives on the (pre-existing) students table, not
	// student_registration_details — only touch it when the caller actually
	// sent a value, and only if a students row already exists for them.
	if in.EnrollmentNo != nil {
		if _, err := r.pool.Exec(ctx, `UPDATE students SET enrollment_no = $1 WHERE user_id = $2`, *in.EnrollmentNo, userID); err != nil {
			return err
		}
	}
	return nil
}

// ── Status lifecycle ────────────────────────────────────────────────────────

// UpdateStatus updates the student's current status and appends a history
// row, in one transaction.
func (r *StudentRegistrationRepository) UpdateStatus(ctx context.Context, studentUserID, status, notes, changedBy string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx, `UPDATE students SET status = $1::student_status WHERE user_id = $2`, status, studentUserID)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		_, err = tx.Exec(ctx, `
			INSERT INTO student_status_history (short_id, student_user_id, status, notes, changed_by)
			VALUES ($1, $2, $3::student_status, NULLIF($4,''), $5)`,
			shortID, studentUserID, status, notes, changedBy,
		)
		if err == nil {
			return tx.Commit(ctx)
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return fmt.Errorf("insert status history: %w", err)
	}
	return fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// GetStatusHistory returns every status-change event for a student, newest first.
func (r *StudentRegistrationRepository) GetStatusHistory(ctx context.Context, studentUserID string) ([]models.StudentStatusHistoryEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT h.id, h.short_id, h.student_user_id, h.status::TEXT, COALESCE(h.notes, ''),
		       h.changed_by, CONCAT(u.first_name, ' ', u.last_name), h.created_at
		FROM student_status_history h
		JOIN users u ON h.changed_by = u.id AND u.deleted_at IS NULL
		WHERE h.student_user_id = $1
		ORDER BY h.created_at DESC`,
		studentUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.StudentStatusHistoryEntry
	for rows.Next() {
		var e models.StudentStatusHistoryEntry
		if err := rows.Scan(&e.ID, &e.ShortID, &e.StudentUserID, &e.Status, &e.Notes, &e.ChangedBy, &e.ChangedByName, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetCurrentStatus returns just the student's current status string.
func (r *StudentRegistrationRepository) GetCurrentStatus(ctx context.Context, studentUserID string) (string, error) {
	var status string
	err := r.pool.QueryRow(ctx, `SELECT status::TEXT FROM students WHERE user_id = $1`, studentUserID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return status, err
}
