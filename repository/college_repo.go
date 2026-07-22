package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

// DefaultCollegeShortID is the short_id of the one permanent row
// representing the Internal EdTech Platform (the company's own direct
// students/batches, as opposed to any external college) — created by
// schema_updates_college_v1.sql and never soft-deleted.
const DefaultCollegeShortID = "DEFAULTCOL"

type CollegeRepository struct {
	pool              *pgxpool.Pool
	defaultCollegeID  atomic.Value // caches the resolved UUID string once found
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
// existing enabled_features map (only the sent keys change), and — when
// schema_updates_college_multitenancy_v2.sql has been applied — also
// upserts the relational college_features rows with enabled_by/enabled_at/
// disabled_at provenance, stamping actorID as whoever enabled/disabled each
// key. The JSONB write always happens (it's the pre-existing column, always
// present) so nothing regresses if the relational tables aren't there yet;
// the relational write is best-effort and only logged on failure.
func (r *CollegeRepository) UpdateFeatures(ctx context.Context, shortID string, features map[string]bool, actorID string) error {
	featuresJSON, err := json.Marshal(features)
	if err != nil {
		return err
	}
	var collegeID string
	err = r.pool.QueryRow(ctx, `
		UPDATE colleges
		SET enabled_features = enabled_features || $2::JSONB, updated_at = NOW()
		WHERE short_id = $1 AND deleted_at IS NULL
		RETURNING id::TEXT`,
		shortID, string(featuresJSON),
	).Scan(&collegeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgx.ErrNoRows
	}
	if err != nil {
		return err
	}

	for code, enabled := range features {
		_, secErr := r.pool.Exec(ctx, `
			INSERT INTO college_features (college_id, feature_id, is_enabled, enabled_by, enabled_at, disabled_at)
			SELECT $1::UUID, pf.id, $2,
			       CASE WHEN $2 THEN $3::UUID ELSE NULL END,
			       CASE WHEN $2 THEN NOW() ELSE NULL END,
			       CASE WHEN $2 THEN NULL ELSE NOW() END
			FROM platform_features pf WHERE pf.code = $4
			ON CONFLICT (college_id, feature_id) DO UPDATE SET
				is_enabled  = EXCLUDED.is_enabled,
				enabled_by  = CASE WHEN EXCLUDED.is_enabled THEN EXCLUDED.enabled_by ELSE college_features.enabled_by END,
				enabled_at  = CASE WHEN EXCLUDED.is_enabled THEN NOW() ELSE college_features.enabled_at END,
				disabled_at = CASE WHEN EXCLUDED.is_enabled THEN NULL ELSE NOW() END,
				updated_at  = NOW()`,
			collegeID, enabled, actorID, code,
		)
		if secErr != nil {
			log.Printf("update relational college_features for college %s, feature %s (migration pending?): %v", shortID, code, secErr)
		}
	}
	return nil
}

// NextRegistrationNo atomically assigns the next sequential registration
// number for a college and advances its counter — race-safe via Postgres's
// row-level lock during the UPDATE itself (a concurrent call on the same
// college blocks until this one commits), so this must never be a separate
// SELECT followed by an UPDATE. ok=false means the migration adding
// next_registration_no hasn't been applied yet (or any other error) —
// callers must skip assigning a registration number entirely in that case,
// never fall back to 0 or a derived count.
func (r *CollegeRepository) NextRegistrationNo(ctx context.Context, collegeID string) (int, bool) {
	var n int
	err := r.pool.QueryRow(ctx, `
		UPDATE colleges SET next_registration_no = next_registration_no + 1
		WHERE id = $1::UUID AND deleted_at IS NULL
		RETURNING next_registration_no - 1`,
		collegeID,
	).Scan(&n)
	return n, err == nil
}

// IsSubscriptionActive reports whether collegeID currently has an active
// subscription. A college with ZERO rows in college_subscriptions has no
// subscription configured at all — treated as unrestricted (true, ok=true),
// not expired, so this feature can be adopted gradually without locking out
// every existing college the moment it's configured for the first time.
// ok=false means the migration adding college_subscriptions hasn't been
// applied yet (or any other error) — callers must fail open (never block
// access) in that case.
func (r *CollegeRepository) IsSubscriptionActive(ctx context.Context, collegeID string) (bool, bool) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM college_subscriptions WHERE college_id = $1::UUID`, collegeID).Scan(&total); err != nil {
		return false, false
	}
	if total == 0 {
		return true, true
	}
	var activeCount int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM college_subscriptions
		WHERE college_id = $1::UUID AND status IN ('active', 'renewed')
		  AND start_date <= CURRENT_DATE AND end_date >= CURRENT_DATE`,
		collegeID,
	).Scan(&activeCount)
	if err != nil {
		return false, false
	}
	return activeCount > 0, true
}

// CreateSubscription records a new subscription period for a college —
// renewals insert a new row rather than mutating the old one, so history is
// preserved.
func (r *CollegeRepository) CreateSubscription(ctx context.Context, collegeID, planID, startDate, endDate, createdBy string) error {
	shortID := util.GenerateShortID()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO college_subscriptions (short_id, college_id, plan_id, start_date, end_date, status, created_by)
		VALUES ($1, $2::UUID, NULLIF($3,''), $4::DATE, $5::DATE, 'active', $6::UUID)`,
		shortID, collegeID, planID, startDate, endDate, createdBy,
	)
	return err
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

// DefaultCollegeID resolves the Internal EdTech Platform's college UUID —
// every direct-platform creation flow (self-registration, admin-provisioned
// students/staff with no college specified) must call this rather than
// leaving college_id unset, so no newly created tenant-owned row is ever
// created with a NULL college_id. Cached after the first successful lookup
// (this row is permanent and never changes id). Returns a clear error if the
// row is somehow missing — callers must NOT treat that as "no college
// needed" and silently proceed with an empty id.
func (r *CollegeRepository) DefaultCollegeID(ctx context.Context) (string, error) {
	if cached, ok := r.defaultCollegeID.Load().(string); ok && cached != "" {
		return cached, nil
	}
	var id string
	err := r.pool.QueryRow(ctx, `SELECT id::TEXT FROM colleges WHERE short_id = $1 AND deleted_at IS NULL`, DefaultCollegeShortID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("internal EdTech Platform college (short_id=%s) not found — has schema_updates_college_v1.sql been applied?", DefaultCollegeShortID)
		}
		return "", err
	}
	r.defaultCollegeID.Store(id)
	return id, nil
}

// GetOrganizationType reads a college's organization_type via an isolated
// query, separate from collegeBaseSelect (schema_updates_college_multitenancy_v2.sql
// hasn't necessarily been applied everywhere yet) — ok=false means the
// migration is pending or the column doesn't exist; callers must not treat
// that as "internal" or "college," just as "unknown."
func (r *CollegeRepository) GetOrganizationType(ctx context.Context, collegeID string) (string, bool) {
	var orgType string
	err := r.pool.QueryRow(ctx, `SELECT organization_type FROM colleges WHERE id = $1::UUID AND deleted_at IS NULL`, collegeID).Scan(&orgType)
	return orgType, err == nil
}

// GetEnabledFeatures reads a college's feature map from the relational
// college_features table (schema_updates_college_multitenancy_v2.sql) —
// code -> is_enabled, for every feature that has a row (enabled or
// disabled) for this college. ok=false means either that migration hasn't
// been applied yet, or this college has zero rows there (not backfilled) —
// in both cases the caller must fall back to the legacy enabled_features
// JSONB, never treat ok=false as "everything disabled."
func (r *CollegeRepository) GetEnabledFeatures(ctx context.Context, collegeID string) (map[string]bool, bool) {
	rows, err := r.pool.Query(ctx, `
		SELECT pf.code, cf.is_enabled
		FROM college_features cf
		JOIN platform_features pf ON pf.id = cf.feature_id
		WHERE cf.college_id = $1::UUID`,
		collegeID,
	)
	if err != nil {
		return nil, false
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var code string
		var enabled bool
		if err := rows.Scan(&code, &enabled); err != nil {
			return nil, false
		}
		out[code] = enabled
	}
	if err := rows.Err(); err != nil {
		return nil, false
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// HasFeature reports whether the given feature key is enabled for a
// college. Used by the RequireFeature middleware — a missing college row
// (e.g. a stale/invalid college_id) is treated as "not enabled" (fail closed).
// Reads the relational college_features table first; falls back to the
// legacy enabled_features JSONB when that table isn't populated yet for
// this college (see GetEnabledFeatures).
func (r *CollegeRepository) HasFeature(ctx context.Context, collegeID, featureKey string) (bool, error) {
	if features, ok := r.GetEnabledFeatures(ctx, collegeID); ok {
		return features[featureKey], nil
	}

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
