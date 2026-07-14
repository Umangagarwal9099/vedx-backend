package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type CertificateRepository struct {
	pool *pgxpool.Pool
}

func NewCertificateRepository(pool *pgxpool.Pool) *CertificateRepository {
	return &CertificateRepository{pool: pool}
}

const certificateBaseSelect = `
	SELECT cert.id, cert.short_id, cert.certificate_number,
	       cert.student_id, CONCAT(u.first_name, ' ', u.last_name),
	       cert.batch_id, b.short_id, b.batch_number,
	       cert.course_id, c.name,
	       cert.final_score, cert.final_rank,
	       COALESCE(cert.issued_by::TEXT, ''), cert.issued_at, cert.revoked_at,
	       cert.created_at, cert.updated_at
	FROM certificates cert
	JOIN users   u ON cert.student_id = u.id
	JOIN batches b ON cert.batch_id   = b.id
	JOIN courses c ON cert.course_id  = c.id`

func scanCertificate(row pgx.Row) (models.Certificate, error) {
	var cert models.Certificate
	err := row.Scan(
		&cert.ID, &cert.ShortID, &cert.CertificateNumber,
		&cert.StudentID, &cert.StudentName,
		&cert.BatchID, &cert.BatchShortID, &cert.BatchNumber,
		&cert.CourseID, &cert.CourseName,
		&cert.FinalScore, &cert.FinalRank,
		&cert.IssuedBy, &cert.IssuedAt, &cert.RevokedAt,
		&cert.CreatedAt, &cert.UpdatedAt,
	)
	return cert, err
}

// Issue records a certificate for a student's completion of a batch,
// snapshotting their final_score/final_rank from student_enrollments at
// issuance time. Re-issuing (same student+batch) refreshes the snapshot,
// issued_at, and clears any prior revocation.
func (r *CertificateRepository) Issue(ctx context.Context, studentID, batchID, courseID, issuedBy string) (*models.Certificate, error) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		certNumber := "CERT-" + util.GenerateShortID()

		cert, err := scanCertificate(r.pool.QueryRow(ctx, fmt.Sprintf(`
			WITH ins AS (
				INSERT INTO certificates (
					short_id, certificate_number, student_id, batch_id, course_id,
					final_score, final_rank, issued_by, issued_at, revoked_at
				)
				SELECT $1, $2, $3::UUID, $4::UUID, $5::UUID,
				       se.final_score, se.final_rank, $6::UUID, NOW(), NULL
				FROM student_enrollments se
				WHERE se.student_id = $3::UUID AND se.batch_id = $4::UUID
				ON CONFLICT (student_id, batch_id) DO UPDATE SET
					final_score = EXCLUDED.final_score, final_rank = EXCLUDED.final_rank,
					issued_by = EXCLUDED.issued_by, issued_at = NOW(), revoked_at = NULL, updated_at = NOW()
				RETURNING *
			)
			%s WHERE cert.id = (SELECT id FROM ins)`, certificateBaseSelect),
			shortID, certNumber, studentID, batchID, courseID, issuedBy,
		))
		if err == nil {
			return &cert, nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("no enrollment found for this student in this batch")
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("issue certificate: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// GetForBatch returns every certificate issued for a batch.
func (r *CertificateRepository) GetForBatch(ctx context.Context, batchID string) ([]models.Certificate, error) {
	rows, err := r.pool.Query(ctx, certificateBaseSelect+" WHERE cert.batch_id = $1::UUID ORDER BY cert.issued_at DESC", batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Certificate
	for rows.Next() {
		cert, err := scanCertificate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cert)
	}
	return out, rows.Err()
}

// GetAll returns every issued certificate, newest first — or, when mentorID
// is non-empty, only certificates for batches that mentor manages.
func (r *CertificateRepository) GetAll(ctx context.Context, mentorID string) ([]models.Certificate, error) {
	q := certificateBaseSelect
	args := []interface{}{}
	if mentorID != "" {
		q += " WHERE b.batch_manager_id = $1::UUID OR b.additional_manager_id = $1::UUID"
		args = append(args, mentorID)
	}
	q += " ORDER BY cert.issued_at DESC LIMIT 500"

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Certificate
	for rows.Next() {
		cert, err := scanCertificate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cert)
	}
	return out, rows.Err()
}

// GetForStudent returns every certificate issued to a student, across every batch.
func (r *CertificateRepository) GetForStudent(ctx context.Context, studentID string) ([]models.Certificate, error) {
	rows, err := r.pool.Query(ctx, certificateBaseSelect+" WHERE cert.student_id = $1::UUID ORDER BY cert.issued_at DESC", studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Certificate
	for rows.Next() {
		cert, err := scanCertificate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cert)
	}
	return out, rows.Err()
}

// VerifyByNumber looks up a certificate by its public certificate_number for
// the unauthenticated verification endpoint. Returns nil (not an error) if
// not found or revoked — the caller reports "invalid" either way.
func (r *CertificateRepository) VerifyByNumber(ctx context.Context, certificateNumber string) (*models.Certificate, error) {
	cert, err := scanCertificate(r.pool.QueryRow(ctx,
		certificateBaseSelect+" WHERE cert.certificate_number = $1 AND cert.revoked_at IS NULL",
		certificateNumber,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

// Revoke marks a certificate revoked (soft — kept for history/audit).
func (r *CertificateRepository) Revoke(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		"UPDATE certificates SET revoked_at = NOW(), updated_at = NOW() WHERE short_id = $1 AND revoked_at IS NULL",
		shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
