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

type ModuleRepository struct {
	pool *pgxpool.Pool
}

func NewModuleRepository(pool *pgxpool.Pool) *ModuleRepository {
	return &ModuleRepository{pool: pool}
}

const moduleBaseSelect = `
	SELECT id, short_id, module_name, module_branch,
	       max_view_duration, watch_time_minutes,
	       is_active, created_by, created_at, updated_at
	FROM modules`

// Create inserts a module and returns the full record. Retries on short_id collision.
func (r *ModuleRepository) Create(ctx context.Context, in models.CreateModuleInput, createdBy string) (*models.Module, error) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		var m models.Module
		err := r.pool.QueryRow(ctx, `
			INSERT INTO modules
			  (short_id, module_name, module_branch, max_view_duration, watch_time_minutes, created_by)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, short_id, module_name, module_branch,
			          max_view_duration, watch_time_minutes,
			          is_active, created_by, created_at, updated_at`,
			shortID, in.ModuleName, in.ModuleBranch, in.MaxViewDuration,
			in.WatchTimeMinutes, createdBy,
		).Scan(
			&m.ID, &m.ShortID, &m.ModuleName, &m.ModuleBranch,
			&m.MaxViewDuration, &m.WatchTimeMinutes,
			&m.IsActive, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt,
		)
		if err == nil {
			return &m, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return nil, fmt.Errorf("insert module: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAll returns all non-deleted modules ordered newest first.
func (r *ModuleRepository) FindAll(ctx context.Context) ([]models.Module, error) {
	q := moduleBaseSelect + ` WHERE deleted_at IS NULL ORDER BY created_at DESC`
	return r.scanModules(ctx, q)
}

// FindByShortID returns a single non-deleted module.
func (r *ModuleRepository) FindByShortID(ctx context.Context, shortID string) (*models.Module, error) {
	q := moduleBaseSelect + ` WHERE short_id = $1 AND deleted_at IS NULL LIMIT 1`

	var m models.Module
	err := r.pool.QueryRow(ctx, q, shortID).Scan(
		&m.ID, &m.ShortID, &m.ModuleName, &m.ModuleBranch,
		&m.MaxViewDuration, &m.WatchTimeMinutes,
		&m.IsActive, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// Filter returns non-deleted modules matching the provided filter.
func (r *ModuleRepository) Filter(ctx context.Context, f models.ModuleFilter) ([]models.Module, error) {
	conditions := []string{"deleted_at IS NULL"}
	args := []interface{}{}
	i := 1

	if f.ModuleName != "" {
		conditions = append(conditions, fmt.Sprintf("module_name ILIKE $%d", i))
		args = append(args, "%"+f.ModuleName+"%")
		i++
	}
	if f.ModuleBranch != "" {
		conditions = append(conditions, fmt.Sprintf("module_branch ILIKE $%d", i))
		args = append(args, "%"+f.ModuleBranch+"%")
		i++
	}
	if f.MaxViewDuration != "" {
		conditions = append(conditions, fmt.Sprintf("max_view_duration = $%d", i))
		args = append(args, f.MaxViewDuration)
		i++
	}
	if f.IsActive == "true" {
		conditions = append(conditions, "is_active = TRUE")
	} else if f.IsActive == "false" {
		conditions = append(conditions, "is_active = FALSE")
	}

	q := moduleBaseSelect + " WHERE " + strings.Join(conditions, " AND ") + " ORDER BY created_at DESC"
	return r.scanModules(ctx, q, args...)
}

// Update applies a partial update — only non-nil fields are changed.
func (r *ModuleRepository) Update(ctx context.Context, shortID string, in models.UpdateModuleInput) error {
	args := []interface{}{shortID}
	setClauses := []string{}
	i := 2

	if in.ModuleName != nil {
		setClauses = append(setClauses, fmt.Sprintf("module_name = $%d", i))
		args = append(args, *in.ModuleName)
		i++
	}
	if in.ModuleBranch != nil {
		setClauses = append(setClauses, fmt.Sprintf("module_branch = $%d", i))
		args = append(args, *in.ModuleBranch)
		i++
	}
	if in.MaxViewDuration != nil {
		setClauses = append(setClauses, fmt.Sprintf("max_view_duration = $%d", i))
		args = append(args, *in.MaxViewDuration)
		i++
		// switching to unlimited clears watch time
		if *in.MaxViewDuration == "unlimited" {
			setClauses = append(setClauses, "watch_time_minutes = NULL")
		}
	}
	if in.WatchTimeMinutes != nil {
		setClauses = append(setClauses, fmt.Sprintf("watch_time_minutes = $%d", i))
		args = append(args, *in.WatchTimeMinutes)
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

	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf(
		"UPDATE modules SET %s WHERE short_id = $1 AND deleted_at IS NULL",
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

// Delete soft-deletes a module.
func (r *ModuleRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE modules SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ── Sections ──────────────────────────────────────────────────────────────────

const sectionCols = `
	id, short_id, module_id::TEXT,
	section_name, COALESCE(short_description, ''),
	is_prerequisite, is_active,
	created_by::TEXT,
	created_at, updated_at`

// AddSection inserts a new section into the module identified by moduleShortID.
func (r *ModuleRepository) AddSection(ctx context.Context, moduleShortID string, in models.CreateModuleSectionInput, createdBy string) (*models.ModuleSection, error) {
	var moduleID string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM modules WHERE short_id = $1 AND deleted_at IS NULL`, moduleShortID,
	).Scan(&moduleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	if err != nil {
		return nil, err
	}

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		var s models.ModuleSection
		err := r.pool.QueryRow(ctx, fmt.Sprintf(`
			INSERT INTO module_sections
			  (short_id, module_id, section_name, short_description, is_prerequisite, created_by)
			VALUES ($1, $2, $3, NULLIF($4,''), $5, $6::UUID)
			RETURNING %s`, sectionCols),
			shortID, moduleID, in.SectionName, in.ShortDescription, in.IsPrerequisite, createdBy,
		).Scan(
			&s.ID, &s.ShortID, &s.ModuleID,
			&s.SectionName, &s.ShortDescription,
			&s.IsPrerequisite, &s.IsActive,
			&s.CreatedBy,
			&s.CreatedAt, &s.UpdatedAt,
		)
		if err == nil {
			return &s, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return nil, fmt.Errorf("insert module_section: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindSections returns all non-deleted sections for a module.
func (r *ModuleRepository) FindSections(ctx context.Context, moduleShortID string) ([]models.ModuleSection, error) {
	var moduleID string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM modules WHERE short_id = $1 AND deleted_at IS NULL`, moduleShortID,
	).Scan(&moduleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	if err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM module_sections
		WHERE module_id = $1 AND deleted_at IS NULL
		ORDER BY created_at ASC`, sectionCols),
		moduleID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sections := []models.ModuleSection{}
	for rows.Next() {
		var s models.ModuleSection
		if err := rows.Scan(
			&s.ID, &s.ShortID, &s.ModuleID,
			&s.SectionName, &s.ShortDescription,
			&s.IsPrerequisite, &s.IsActive,
			&s.CreatedBy,
			&s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		sections = append(sections, s)
	}
	return sections, rows.Err()
}

// UpdateSection partially updates a section and returns the updated row.
func (r *ModuleRepository) UpdateSection(ctx context.Context, moduleShortID, sectionShortID string, in models.UpdateModuleSectionInput) (*models.ModuleSection, error) {
	var moduleID string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM modules WHERE short_id = $1 AND deleted_at IS NULL`, moduleShortID,
	).Scan(&moduleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	if err != nil {
		return nil, err
	}

	args := []interface{}{sectionShortID, moduleID}
	setClauses := []string{}
	i := 3

	if in.SectionName != nil {
		setClauses = append(setClauses, fmt.Sprintf("section_name = $%d", i))
		args = append(args, *in.SectionName)
		i++
	}
	if in.ShortDescription != nil {
		setClauses = append(setClauses, fmt.Sprintf("short_description = NULLIF($%d,'')", i))
		args = append(args, *in.ShortDescription)
		i++
	}
	if in.IsPrerequisite != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_prerequisite = $%d", i))
		args = append(args, *in.IsPrerequisite)
		i++
	}
	if in.IsActive != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_active = $%d", i))
		args = append(args, *in.IsActive)
		i++
	}

	if len(setClauses) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}
	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf(
		`UPDATE module_sections SET %s WHERE short_id = $1 AND module_id = $2 AND deleted_at IS NULL RETURNING %s`,
		strings.Join(setClauses, ", "),
		sectionCols,
	)
	var s models.ModuleSection
	err = r.pool.QueryRow(ctx, q, args...).Scan(
		&s.ID, &s.ShortID, &s.ModuleID,
		&s.SectionName, &s.ShortDescription,
		&s.IsPrerequisite, &s.IsActive,
		&s.CreatedBy,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	return &s, err
}

// DeleteSection soft-deletes a section.
func (r *ModuleRepository) DeleteSection(ctx context.Context, moduleShortID, sectionShortID string) error {
	var moduleID string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM modules WHERE short_id = $1 AND deleted_at IS NULL`, moduleShortID,
	).Scan(&moduleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgx.ErrNoRows
	}
	if err != nil {
		return err
	}

	result, err := r.pool.Exec(ctx,
		`UPDATE module_sections SET deleted_at = NOW() WHERE short_id = $1 AND module_id = $2 AND deleted_at IS NULL`,
		sectionShortID, moduleID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ── Section Materials ──────────────────────────────────────────────────────────

const materialBaseSelect = `
	SELECT id, short_id, section_id, material_name, material_type,
	       file_url, max_views, max_views_count,
	       is_prerequisite, enable_downloads, allow_access_on,
	       is_active, created_by, created_at, updated_at
	FROM section_materials`

// CreateMaterial inserts a material under the given section (looked up by sectionShortID).
func (r *ModuleRepository) CreateMaterial(ctx context.Context, sectionShortID, createdBy string, in models.CreateMaterialInput) (*models.SectionMaterial, error) {
	var sectionID string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM module_sections WHERE short_id = $1 AND deleted_at IS NULL`, sectionShortID,
	).Scan(&sectionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	if err != nil {
		return nil, err
	}

	for {
		sid := util.GenerateShortID()
		var m models.SectionMaterial
		err = r.pool.QueryRow(ctx, `
			INSERT INTO section_materials
			       (short_id, section_id, material_name, material_type,
			        file_url, max_views, max_views_count,
			        is_prerequisite, enable_downloads, allow_access_on,
			        created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			RETURNING id, short_id, section_id, material_name, material_type,
			          file_url, max_views, max_views_count,
			          is_prerequisite, enable_downloads, allow_access_on,
			          is_active, created_by, created_at, updated_at`,
			sid, sectionID, in.MaterialName, in.MaterialType,
			in.FileURL, in.MaxViews, in.MaxViewsCount,
			in.IsPrerequisite, in.EnableDownloads, in.AllowAccessOn,
			createdBy,
		).Scan(
			&m.ID, &m.ShortID, &m.SectionID, &m.MaterialName, &m.MaterialType,
			&m.FileURL, &m.MaxViews, &m.MaxViewsCount,
			&m.IsPrerequisite, &m.EnableDownloads, &m.AllowAccessOn,
			&m.IsActive, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt,
		)
		if err == nil {
			return &m, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue // short_id collision — retry
		}
		return nil, err
	}
}

// GetMaterials returns all active materials for a section.
func (r *ModuleRepository) GetMaterials(ctx context.Context, sectionShortID string) ([]models.SectionMaterial, error) {
	var sectionID string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM module_sections WHERE short_id = $1 AND deleted_at IS NULL`, sectionShortID,
	).Scan(&sectionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	if err != nil {
		return nil, err
	}

	q := materialBaseSelect + ` WHERE section_id = $1 AND deleted_at IS NULL ORDER BY created_at ASC`
	rows, err := r.pool.Query(ctx, q, sectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var materials []models.SectionMaterial
	for rows.Next() {
		var m models.SectionMaterial
		if err := rows.Scan(
			&m.ID, &m.ShortID, &m.SectionID, &m.MaterialName, &m.MaterialType,
			&m.FileURL, &m.MaxViews, &m.MaxViewsCount,
			&m.IsPrerequisite, &m.EnableDownloads, &m.AllowAccessOn,
			&m.IsActive, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		materials = append(materials, m)
	}
	return materials, rows.Err()
}

// UpdateMaterial applies partial updates to a material.
func (r *ModuleRepository) UpdateMaterial(ctx context.Context, sectionShortID, materialShortID string, in models.UpdateMaterialInput) (*models.SectionMaterial, error) {
	var sectionID string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM module_sections WHERE short_id = $1 AND deleted_at IS NULL`, sectionShortID,
	).Scan(&sectionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	if err != nil {
		return nil, err
	}

	setClauses := []string{"updated_at = NOW()"}
	args := []interface{}{}
	i := 1

	if in.MaterialName != nil {
		setClauses = append(setClauses, fmt.Sprintf("material_name = $%d", i))
		args = append(args, *in.MaterialName)
		i++
	}
	if in.MaterialType != nil {
		setClauses = append(setClauses, fmt.Sprintf("material_type = $%d", i))
		args = append(args, *in.MaterialType)
		i++
	}
	if in.FileURL != nil {
		setClauses = append(setClauses, fmt.Sprintf("file_url = $%d", i))
		args = append(args, *in.FileURL)
		i++
	}
	if in.MaxViews != nil {
		setClauses = append(setClauses, fmt.Sprintf("max_views = $%d", i))
		args = append(args, *in.MaxViews)
		i++
	}
	if in.MaxViewsCount != nil {
		setClauses = append(setClauses, fmt.Sprintf("max_views_count = $%d", i))
		args = append(args, *in.MaxViewsCount)
		i++
	}
	if in.IsPrerequisite != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_prerequisite = $%d", i))
		args = append(args, *in.IsPrerequisite)
		i++
	}
	if in.EnableDownloads != nil {
		setClauses = append(setClauses, fmt.Sprintf("enable_downloads = $%d", i))
		args = append(args, *in.EnableDownloads)
		i++
	}
	if in.AllowAccessOn != nil {
		setClauses = append(setClauses, fmt.Sprintf("allow_access_on = $%d", i))
		args = append(args, *in.AllowAccessOn)
		i++
	}
	if in.IsActive != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_active = $%d", i))
		args = append(args, *in.IsActive)
		i++
	}

	args = append(args, materialShortID, sectionID)
	q := fmt.Sprintf(`
		UPDATE section_materials SET %s
		WHERE short_id = $%d AND section_id = $%d AND deleted_at IS NULL
		RETURNING id, short_id, section_id, material_name, material_type,
		          file_url, max_views, max_views_count,
		          is_prerequisite, enable_downloads, allow_access_on,
		          is_active, created_by, created_at, updated_at`,
		strings.Join(setClauses, ", "), i, i+1,
	)

	var m models.SectionMaterial
	err = r.pool.QueryRow(ctx, q, args...).Scan(
		&m.ID, &m.ShortID, &m.SectionID, &m.MaterialName, &m.MaterialType,
		&m.FileURL, &m.MaxViews, &m.MaxViewsCount,
		&m.IsPrerequisite, &m.EnableDownloads, &m.AllowAccessOn,
		&m.IsActive, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pgx.ErrNoRows
	}
	return &m, err
}

// DeleteMaterial soft-deletes a material.
func (r *ModuleRepository) DeleteMaterial(ctx context.Context, sectionShortID, materialShortID string) error {
	var sectionID string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM module_sections WHERE short_id = $1 AND deleted_at IS NULL`, sectionShortID,
	).Scan(&sectionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgx.ErrNoRows
	}
	if err != nil {
		return err
	}

	result, err := r.pool.Exec(ctx,
		`UPDATE section_materials SET deleted_at = NOW() WHERE short_id = $1 AND section_id = $2 AND deleted_at IS NULL`,
		materialShortID, sectionID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *ModuleRepository) scanModules(ctx context.Context, q string, args ...interface{}) ([]models.Module, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var modules []models.Module
	for rows.Next() {
		var m models.Module
		if err := rows.Scan(
			&m.ID, &m.ShortID, &m.ModuleName, &m.ModuleBranch,
			&m.MaxViewDuration, &m.WatchTimeMinutes,
			&m.IsActive, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		modules = append(modules, m)
	}
	return modules, rows.Err()
}
