package repository

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type CourseRepository struct {
	pool *pgxpool.Pool
}

func NewCourseRepository(pool *pgxpool.Pool) *CourseRepository {
	return &CourseRepository{pool: pool}
}

// courseSelectCols is the canonical column list for all course SELECTs.
const courseSelectCols = `
	id, short_id, COALESCE(college_id::TEXT, ''), name,
	COALESCE(description,''), COALESCE(thumbnail,''),
	COALESCE(overview,''), COALESCE(objectives,'{}'), COALESCE(requirements,'{}'),
	COALESCE(instructor,''), COALESCE(duration,''), COALESCE(level,''), COALESCE(category,''),
	is_active, created_by, created_at, updated_at`

func scanCourse(row pgx.Row) (models.Course, error) {
	var c models.Course
	err := row.Scan(
		&c.ID, &c.ShortID, &c.CollegeID, &c.Name, &c.Description, &c.Thumbnail,
		&c.Overview, &c.Objectives, &c.Requirements,
		&c.Instructor, &c.Duration, &c.Level, &c.Category,
		&c.IsActive, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
	)
	return c, err
}

// Create inserts a new course, retrying up to 3 times on short_id collision.
// collegeID must never be empty — resolved by the caller (see
// controller.resolveTargetCollege) so no newly created course is ever left
// with a NULL college_id.
func (r *CourseRepository) Create(ctx context.Context, in models.CreateCourseInput, createdBy, collegeID string) (*models.Course, error) {
	const q = `
		INSERT INTO courses (
			short_id, college_id, name, description, thumbnail,
			overview, objectives, requirements,
			instructor, duration, level, category,
			created_by
		) VALUES ($1,$2::UUID,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),$7,$8,NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),$13)
		RETURNING` + courseSelectCols

	objs := in.Objectives
	if objs == nil {
		objs = []string{}
	}
	reqs := in.Requirements
	if reqs == nil {
		reqs = []string{}
	}

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		c, err := scanCourse(r.pool.QueryRow(ctx, q,
			shortID, collegeID, in.Name, in.Description, in.Thumbnail,
			in.Overview, objs, reqs,
			in.Instructor, in.Duration, in.Level, in.Category,
			createdBy,
		))
		if err == nil {
			// Best-effort — course_scope is new (Stage 5); if the migration
			// isn't applied yet, silently skip rather than fail the whole
			// course creation over an optional classification.
			if in.CourseScope == "global" {
				if _, secErr := r.pool.Exec(ctx, `UPDATE courses SET course_scope = 'global' WHERE id = $1`, c.ID); secErr != nil {
					log.Printf("set course_scope for new course %s (migration pending?): %v", c.ShortID, secErr)
				}
			}
			return &c, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("insert course: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAll returns all non-deleted courses ordered newest first. collegeID
// scopes the list for non-super-admin callers (empty = unscoped).
func (r *CourseRepository) FindAll(ctx context.Context, collegeID string) ([]models.Course, error) {
	q := `SELECT` + courseSelectCols + ` FROM courses WHERE deleted_at IS NULL`
	args := []interface{}{}
	if collegeID != "" {
		q += ` AND college_id = $1::UUID`
		args = append(args, collegeID)
	}
	q += ` ORDER BY created_at DESC`
	return r.scanCourses(ctx, q, args...)
}

// IsAvailableToCollege reports whether courseID may be used by collegeID —
// true when the course is directly owned by that college (course.college_id
// matches), OR the course is course_scope='global' and has an enabled,
// in-window college_courses assignment row for that college. ok=false means
// schema_updates_college_multitenancy_v2.sql's Stage 5 additions
// (course_scope/college_courses) haven't been applied yet (or any other
// error) — callers must treat that as "unknown, don't block batch
// creation," never as "not available," matching every other fail-open
// pattern introduced this feature.
func (r *CourseRepository) IsAvailableToCollege(ctx context.Context, courseID, collegeID string) (bool, bool) {
	var available bool
	err := r.pool.QueryRow(ctx, `
		SELECT
			c.college_id = $2::UUID
			OR (
				c.course_scope = 'global' AND EXISTS (
					SELECT 1 FROM college_courses cc
					WHERE cc.college_id = $2::UUID AND cc.course_id = c.id AND cc.is_enabled = TRUE
					  AND (cc.access_start_date IS NULL OR cc.access_start_date <= CURRENT_DATE)
					  AND (cc.access_end_date IS NULL OR cc.access_end_date >= CURRENT_DATE)
				)
			)
		FROM courses c WHERE c.id = $1::UUID AND c.deleted_at IS NULL`,
		courseID, collegeID,
	).Scan(&available)
	return available, err == nil
}

// AssignToCollege creates or updates a global course's college_courses
// assignment row — the mechanism behind "Super Admin assigns a global
// master course to a college" (spec section 17).
func (r *CourseRepository) AssignToCollege(ctx context.Context, courseID, collegeID, assignedBy string, accessStartDate, accessEndDate *string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO college_courses (college_id, course_id, is_enabled, access_start_date, access_end_date, assigned_by)
		VALUES ($1::UUID, $2::UUID, TRUE, NULLIF($3,'')::DATE, NULLIF($4,'')::DATE, $5::UUID)
		ON CONFLICT (college_id, course_id) DO UPDATE SET
			is_enabled = TRUE, access_start_date = EXCLUDED.access_start_date,
			access_end_date = EXCLUDED.access_end_date, assigned_by = EXCLUDED.assigned_by, updated_at = NOW()`,
		collegeID, courseID, accessStartDate, accessEndDate, assignedBy,
	)
	return err
}

// FindByShortID returns a single non-deleted course by its short_id.
//
// NOTE: deliberately NOT college-scoped — see the identical note on
// BatchRepository.FindByShortID; detail-level hardening is a later phase.
func (r *CourseRepository) FindByShortID(ctx context.Context, shortID string) (*models.Course, error) {
	q := `SELECT` + courseSelectCols + ` FROM courses WHERE short_id = $1 AND deleted_at IS NULL LIMIT 1`
	c, err := scanCourse(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Search returns non-deleted courses whose name or description matches the
// query. collegeID scopes the results for non-super-admin callers (empty =
// unscoped) — same rationale as FindAll, since Search is just another list
// endpoint and would otherwise be a scoping bypass.
func (r *CourseRepository) Search(ctx context.Context, query, collegeID string) ([]models.Course, error) {
	q := `SELECT` + courseSelectCols + `
		FROM courses
		WHERE deleted_at IS NULL AND (name ILIKE $1 OR description ILIKE $1)`
	args := []interface{}{"%" + query + "%"}
	if collegeID != "" {
		q += ` AND college_id = $2::UUID`
		args = append(args, collegeID)
	}
	q += ` ORDER BY created_at DESC`
	return r.scanCourses(ctx, q, args...)
}

// Update applies a partial update — only non-nil / non-empty fields are changed.
func (r *CourseRepository) Update(ctx context.Context, shortID string, in models.UpdateCourseInput) error {
	args := []interface{}{shortID}
	setClauses := []string{}
	i := 2

	addStr := func(col, val string, nullable bool) {
		if nullable {
			setClauses = append(setClauses, fmt.Sprintf("%s = NULLIF($%d,'')", col, i))
		} else {
			setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, i))
		}
		args = append(args, val)
		i++
	}

	if in.Name != nil {
		addStr("name", *in.Name, false)
	}
	if in.Description != nil {
		addStr("description", *in.Description, true)
	}
	if in.Thumbnail != nil {
		addStr("thumbnail", *in.Thumbnail, true)
	}
	if in.Overview != nil {
		addStr("overview", *in.Overview, true)
	}
	if in.Instructor != nil {
		addStr("instructor", *in.Instructor, true)
	}
	if in.Duration != nil {
		addStr("duration", *in.Duration, true)
	}
	if in.Level != nil {
		addStr("level", *in.Level, true)
	}
	if in.Category != nil {
		addStr("category", *in.Category, true)
	}
	if in.Objectives != nil {
		setClauses = append(setClauses, fmt.Sprintf("objectives = $%d", i))
		args = append(args, in.Objectives)
		i++
	}
	if in.Requirements != nil {
		setClauses = append(setClauses, fmt.Sprintf("requirements = $%d", i))
		args = append(args, in.Requirements)
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
		"UPDATE courses SET %s WHERE short_id = $1 AND deleted_at IS NULL",
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

// Delete soft-deletes a course by its short_id.
func (r *CourseRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE courses SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// GetCurriculum returns all modules assigned to a course with their sections and materials.
func (r *CourseRepository) GetCurriculum(ctx context.Context, courseShortID string) ([]models.ModuleWithSections, error) {
	const q = `
		SELECT
		  m.short_id, m.module_name, m.module_branch,
		  m.max_view_duration, m.watch_time_minutes, m.is_active, cm.order_index,
		  ms.short_id, ms.section_name, COALESCE(ms.short_description,''), ms.is_prerequisite, ms.is_active,
		  sm.short_id, sm.material_name, sm.material_type::TEXT,
		  sm.file_url, sm.enable_downloads, sm.is_prerequisite, sm.is_active
		FROM course_modules cm
		JOIN modules m ON m.id = cm.module_id AND m.deleted_at IS NULL
		LEFT JOIN module_sections ms ON ms.module_id = m.id AND ms.deleted_at IS NULL AND ms.is_active = TRUE
		LEFT JOIN section_materials sm ON sm.section_id = ms.id AND sm.deleted_at IS NULL AND sm.is_active = TRUE
		WHERE cm.course_id = (SELECT id FROM courses WHERE short_id = $1 AND deleted_at IS NULL)
		ORDER BY cm.order_index, ms.created_at, sm.created_at`

	rows, err := r.pool.Query(ctx, q, courseShortID)
	if err != nil {
		return nil, fmt.Errorf("GetCurriculum query: %w", err)
	}
	defer rows.Close()

	type moduleKey = string
	type sectionKey = string
	moduleOrder := []moduleKey{}
	moduleMap := map[moduleKey]*models.ModuleWithSections{}
	sectionOrder := map[moduleKey][]sectionKey{}
	sectionMap := map[sectionKey]*models.SectionSummary{}

	for rows.Next() {
		var (
			mShortID, mName, mBranch, mMaxView string
			mWatch                              *int
			mActive                             bool
			mOrder                              int
			secShortID, secName, secDesc        *string
			secPrereq, secActive                *bool
			matShortID, matName, matType        *string
			matURL                              *string
			matDownload, matPrereq              *bool
			matActive                           *bool
		)
		if err := rows.Scan(
			&mShortID, &mName, &mBranch, &mMaxView, &mWatch, &mActive, &mOrder,
			&secShortID, &secName, &secDesc, &secPrereq, &secActive,
			&matShortID, &matName, &matType, &matURL, &matDownload, &matPrereq, &matActive,
		); err != nil {
			return nil, err
		}

		if _, exists := moduleMap[mShortID]; !exists {
			moduleOrder = append(moduleOrder, mShortID)
			moduleMap[mShortID] = &models.ModuleWithSections{
				ShortID: mShortID, ModuleName: mName, ModuleBranch: mBranch,
				MaxViewDuration: mMaxView, WatchTimeMinutes: mWatch,
				IsActive: mActive, OrderIndex: mOrder,
				Sections: []models.SectionSummary{},
			}
			sectionOrder[mShortID] = []sectionKey{}
		}

		if secShortID == nil {
			continue
		}
		if _, exists := sectionMap[*secShortID]; !exists {
			sectionOrder[mShortID] = append(sectionOrder[mShortID], *secShortID)
			sectionMap[*secShortID] = &models.SectionSummary{
				ShortID: *secShortID, SectionName: *secName,
				ShortDescription: *secDesc, IsPrerequisite: *secPrereq,
				IsActive: *secActive, Materials: []models.MaterialSummary{},
			}
		}

		if matShortID == nil {
			continue
		}
		sectionMap[*secShortID].Materials = append(sectionMap[*secShortID].Materials, models.MaterialSummary{
			ShortID: *matShortID, MaterialName: *matName, MaterialType: *matType,
			FileURL: matURL, EnableDownloads: *matDownload,
			IsPrerequisite: *matPrereq, IsActive: *matActive,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]models.ModuleWithSections, 0, len(moduleOrder))
	for _, mID := range moduleOrder {
		mod := moduleMap[mID]
		for _, sID := range sectionOrder[mID] {
			mod.Sections = append(mod.Sections, *sectionMap[sID])
		}
		result = append(result, *mod)
	}
	return result, nil
}

// AssignModule links a module to a course.
func (r *CourseRepository) AssignModule(ctx context.Context, courseShortID, moduleShortID string, orderIndex int) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO course_modules (course_id, module_id, order_index)
		SELECT c.id, m.id, $3
		FROM courses c, modules m
		WHERE c.short_id = $1 AND c.deleted_at IS NULL
		  AND m.short_id = $2 AND m.deleted_at IS NULL
		ON CONFLICT (course_id, module_id) DO UPDATE SET order_index = EXCLUDED.order_index`,
		courseShortID, moduleShortID, orderIndex,
	)
	return err
}

// UnassignModule removes a module from a course.
func (r *CourseRepository) UnassignModule(ctx context.Context, courseShortID, moduleShortID string) error {
	result, err := r.pool.Exec(ctx, `
		DELETE FROM course_modules
		WHERE course_id = (SELECT id FROM courses WHERE short_id = $1 AND deleted_at IS NULL)
		  AND module_id = (SELECT id FROM modules WHERE short_id = $2 AND deleted_at IS NULL)`,
		courseShortID, moduleShortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *CourseRepository) scanCourses(ctx context.Context, q string, args ...interface{}) ([]models.Course, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var courses []models.Course
	for rows.Next() {
		var c models.Course
		if err := rows.Scan(
			&c.ID, &c.ShortID, &c.CollegeID, &c.Name, &c.Description, &c.Thumbnail,
			&c.Overview, &c.Objectives, &c.Requirements,
			&c.Instructor, &c.Duration, &c.Level, &c.Category,
			&c.IsActive, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if c.Objectives == nil {
			c.Objectives = []string{}
		}
		if c.Requirements == nil {
			c.Requirements = []string{}
		}
		courses = append(courses, c)
	}
	return courses, rows.Err()
}
