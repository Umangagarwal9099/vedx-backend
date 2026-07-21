package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type CollegeRepository struct {
	pool *pgxpool.Pool
}

func NewCollegeRepository(pool *pgxpool.Pool) *CollegeRepository {
	return &CollegeRepository{pool: pool}
}

const collegeBaseSelect = `
	SELECT c.id, c.short_id, c.name, c.code, COALESCE(c.logo_url,''),
	       COALESCE(c.contact_person,''), COALESCE(c.contact_email,''), COALESCE(c.contact_phone,''),
	       COALESCE(c.address,''),
	       COALESCE(c.subscription_start_date::TEXT,''), COALESCE(c.subscription_end_date::TEXT,''),
	       c.max_students, c.max_employees, c.status, c.enabled_features,
	       (SELECT COUNT(*) FROM users u WHERE u.college_id = c.id AND u.role = 'student' AND u.deleted_at IS NULL),
	       (SELECT COUNT(*) FROM users u WHERE u.college_id = c.id AND u.role IN ('employee','team_lead','mentor') AND u.deleted_at IS NULL),
	       c.created_at, c.updated_at
	FROM colleges c`

func scanCollege(row pgx.Row) (models.College, error) {
	var col models.College
	var featuresJSON []byte
	err := row.Scan(
		&col.ID, &col.ShortID, &col.Name, &col.Code, &col.LogoURL,
		&col.ContactPerson, &col.ContactEmail, &col.ContactPhone,
		&col.Address,
		&col.SubscriptionStartDate, &col.SubscriptionEndDate,
		&col.MaxStudents, &col.MaxEmployees, &col.Status, &featuresJSON,
		&col.StudentCount, &col.EmployeeCount,
		&col.CreatedAt, &col.UpdatedAt,
	)
	if err != nil {
		return col, err
	}
	if len(featuresJSON) > 0 {
		if err := json.Unmarshal(featuresJSON, &col.EnabledFeatures); err != nil {
			return col, err
		}
	}
	if col.EnabledFeatures == nil {
		col.EnabledFeatures = map[string]bool{}
	}
	return col, nil
}

// Create inserts a college. Retries on short_id collision; code must be
// globally unique (returns a friendly error on conflict).
func (r *CollegeRepository) Create(ctx context.Context, in models.CreateCollegeInput, createdBy string) (*models.College, error) {
	features := in.EnabledFeatures
	if features == nil {
		features = map[string]bool{}
	}
	featuresJSON, err := json.Marshal(features)
	if err != nil {
		return nil, err
	}

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		var id string
		err := r.pool.QueryRow(ctx, `
			INSERT INTO colleges (
				short_id, name, code, logo_url, contact_person, contact_email, contact_phone, address,
				subscription_start_date, subscription_end_date, max_students, max_employees,
				enabled_features, created_by
			) VALUES (
				$1, $2, $3, NULLIF($4,''), NULLIF($5,''), NULLIF($6,''), NULLIF($7,''), NULLIF($8,''),
				NULLIF($9,'')::DATE, NULLIF($10,'')::DATE, $11, $12,
				$13::JSONB, $14
			) RETURNING id`,
			shortID, in.Name, in.Code, in.LogoURL, in.ContactPerson, in.ContactEmail, in.ContactPhone, in.Address,
			in.SubscriptionStartDate, in.SubscriptionEndDate, in.MaxStudents, in.MaxEmployees,
			string(featuresJSON), createdBy,
		).Scan(&id)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				if pgErr.ConstraintName == "colleges_code_key" {
					return nil, fmt.Errorf("a college with code %q already exists", in.Code)
				}
				continue // short_id collision, retry
			}
			return nil, err
		}
		return r.FindByID(ctx, id)
	}
	return nil, errors.New("could not generate a unique short_id after 3 attempts")
}

func (r *CollegeRepository) FindByID(ctx context.Context, id string) (*models.College, error) {
	c, err := scanCollege(r.pool.QueryRow(ctx, collegeBaseSelect+" WHERE c.id = $1 AND c.deleted_at IS NULL", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *CollegeRepository) FindByShortID(ctx context.Context, shortID string) (*models.College, error) {
	c, err := scanCollege(r.pool.QueryRow(ctx, collegeBaseSelect+" WHERE c.short_id = $1 AND c.deleted_at IS NULL", shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *CollegeRepository) FindAll(ctx context.Context) ([]models.College, error) {
	rows, err := r.pool.Query(ctx, collegeBaseSelect+" WHERE c.deleted_at IS NULL ORDER BY c.created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.College
	for rows.Next() {
		c, err := scanCollege(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Update applies a partial update — only non-nil fields are changed.
func (r *CollegeRepository) Update(ctx context.Context, shortID string, in models.UpdateCollegeInput) error {
	args := []interface{}{shortID}
	setClauses := []string{"updated_at = NOW()"}
	i := 2

	add := func(clause string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf(clause, i))
		args = append(args, val)
		i++
	}

	if in.Name != nil {
		add("name = $%d", *in.Name)
	}
	if in.LogoURL != nil {
		add("logo_url = NULLIF($%d,'')", *in.LogoURL)
	}
	if in.ContactPerson != nil {
		add("contact_person = NULLIF($%d,'')", *in.ContactPerson)
	}
	if in.ContactEmail != nil {
		add("contact_email = NULLIF($%d,'')", *in.ContactEmail)
	}
	if in.ContactPhone != nil {
		add("contact_phone = NULLIF($%d,'')", *in.ContactPhone)
	}
	if in.Address != nil {
		add("address = NULLIF($%d,'')", *in.Address)
	}
	if in.SubscriptionStartDate != nil {
		add("subscription_start_date = NULLIF($%d,'')::DATE", *in.SubscriptionStartDate)
	}
	if in.SubscriptionEndDate != nil {
		add("subscription_end_date = NULLIF($%d,'')::DATE", *in.SubscriptionEndDate)
	}
	if in.MaxStudents != nil {
		add("max_students = $%d", *in.MaxStudents)
	}
	if in.MaxEmployees != nil {
		add("max_employees = $%d", *in.MaxEmployees)
	}
	if in.Status != nil {
		add("status = $%d", *in.Status)
	}

	if len(setClauses) == 1 {
		return errors.New("no fields to update")
	}

	q := fmt.Sprintf("UPDATE colleges SET %s WHERE short_id = $1 AND deleted_at IS NULL", strings.Join(setClauses, ", "))
	tag, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// UpdateFeatures merges the given feature toggles into the college's
// existing enabled_features map (only the sent keys change).
func (r *CollegeRepository) UpdateFeatures(ctx context.Context, shortID string, features map[string]bool) error {
	featuresJSON, err := json.Marshal(features)
	if err != nil {
		return err
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE colleges
		SET enabled_features = enabled_features || $2::JSONB, updated_at = NOW()
		WHERE short_id = $1 AND deleted_at IS NULL`,
		shortID, string(featuresJSON),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// Delete soft-deletes a college.
func (r *CollegeRepository) Delete(ctx context.Context, shortID string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE colleges SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// HasFeature reports whether the given feature key is enabled for a
// college. Used by the RequireFeature middleware — a missing college row
// (e.g. a stale/invalid college_id) is treated as "not enabled" (fail closed).
func (r *CollegeRepository) HasFeature(ctx context.Context, collegeID, featureKey string) (bool, error) {
	var enabled bool
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE((enabled_features->>$2)::boolean, false)
		FROM colleges WHERE id = $1::UUID AND deleted_at IS NULL`,
		collegeID, featureKey,
	).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return enabled, err
}
