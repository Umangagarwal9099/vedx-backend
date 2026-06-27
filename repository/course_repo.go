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

type CourseRepository struct {
	pool *pgxpool.Pool
}

func NewCourseRepository(pool *pgxpool.Pool) *CourseRepository {
	return &CourseRepository{pool: pool}
}

// courseSelectCols is the canonical column list for all course SELECTs.
const courseSelectCols = `
	id, short_id, name,
	COALESCE(description,''), COALESCE(thumbnail,''),
	COALESCE(overview,''), COALESCE(objectives,'{}'), COALESCE(requirements,'{}'),
	COALESCE(instructor,''), COALESCE(duration,''), COALESCE(level,''), COALESCE(category,''),
	is_active, created_by, created_at, updated_at`

func scanCourse(row pgx.Row) (models.Course, error) {
	var c models.Course
	err := row.Scan(
		&c.ID, &c.ShortID, &c.Name, &c.Description, &c.Thumbnail,
		&c.Overview, &c.Objectives, &c.Requirements,
		&c.Instructor, &c.Duration, &c.Level, &c.Category,
		&c.IsActive, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
	)
	return c, err
}

// Create inserts a new course, retrying up to 3 times on short_id collision.
func (r *CourseRepository) Create(ctx context.Context, in models.CreateCourseInput, createdBy string) (*models.Course, error) {
	const q = `
		INSERT INTO courses (
			short_id, name, description, thumbnail,
			overview, objectives, requirements,
			instructor, duration, level, category,
			created_by
		) VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),$6,$7,NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),$12)
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
			shortID, in.Name, in.Description, in.Thumbnail,
			in.Overview, objs, reqs,
			in.Instructor, in.Duration, in.Level, in.Category,
			createdBy,
		))
		if err == nil {
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

// FindAll returns all non-deleted courses ordered newest first.
func (r *CourseRepository) FindAll(ctx context.Context) ([]models.Course, error) {
	q := `SELECT` + courseSelectCols + ` FROM courses WHERE deleted_at IS NULL ORDER BY created_at DESC`
	return r.scanCourses(ctx, q)
}

// FindByShortID returns a single non-deleted course by its short_id.
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

// Search returns non-deleted courses whose name or description matches the query.
func (r *CourseRepository) Search(ctx context.Context, query string) ([]models.Course, error) {
	q := `SELECT` + courseSelectCols + `
		FROM courses
		WHERE deleted_at IS NULL AND (name ILIKE $1 OR description ILIKE $1)
		ORDER BY created_at DESC`
	return r.scanCourses(ctx, q, "%"+query+"%")
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
			&c.ID, &c.ShortID, &c.Name, &c.Description, &c.Thumbnail,
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
