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

type ProjectRepository struct {
	pool *pgxpool.Pool
}

func NewProjectRepository(pool *pgxpool.Pool) *ProjectRepository {
	return &ProjectRepository{pool: pool}
}

// ── Projects ─────────────────────────────────────────────────────────────────

const projectBaseSelect = `
	SELECT p.id, p.short_id, p.title,
	       COALESCE(p.problem_statement,''), COALESCE(p.requirements,''),
	       COALESCE(p.expected_deliverables,''), COALESCE(p.evaluation_criteria,''),
	       COALESCE(p.reference_files, '{}'),
	       p.category::TEXT, p.is_team_project,
	       b.short_id, b.batch_number,
	       COALESCE(m.short_id, ''), COALESCE(m.module_name, ''),
	       p.max_marks, COALESCE(p.start_date::TEXT, ''), p.final_deadline,
	       p.allowed_submission_types, p.status::TEXT,
	       p.created_by, CONCAT(u.first_name, ' ', u.last_name),
	       p.created_at, p.updated_at
	FROM projects p
	JOIN batches b ON p.batch_id   = b.id
	JOIN users   u ON p.created_by = u.id AND u.deleted_at IS NULL
	LEFT JOIN modules m ON p.module_id = m.id AND m.deleted_at IS NULL`

func scanProject(row pgx.Row) (models.Project, error) {
	var p models.Project
	err := row.Scan(
		&p.ID, &p.ShortID, &p.Title,
		&p.ProblemStatement, &p.Requirements,
		&p.ExpectedDeliverables, &p.EvaluationCriteria,
		&p.ReferenceFiles,
		&p.Category, &p.IsTeamProject,
		&p.BatchShortID, &p.BatchNumber,
		&p.ModuleShortID, &p.ModuleName,
		&p.MaxMarks, &p.StartDate, &p.FinalDeadline,
		&p.AllowedSubmissionTypes, &p.Status,
		&p.CreatedBy, &p.CreatedByName,
		&p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}

func (r *ProjectRepository) Create(ctx context.Context, in models.CreateProjectInput, createdBy string) (*models.Project, error) {
	status := in.Status
	if status == "" {
		status = "draft"
	}

	// See the comment on assessment_repo.go's Create for why this reads FROM
	// ins rather than FROM projects.
	insSelect := strings.Replace(projectBaseSelect, "FROM projects p", "FROM ins p", 1)

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		p, err := scanProject(r.pool.QueryRow(ctx, fmt.Sprintf(`
			WITH ins AS (
				INSERT INTO projects (
					short_id, title, problem_statement, requirements, expected_deliverables,
					evaluation_criteria, reference_files, category, is_team_project,
					batch_id, module_id, max_marks, start_date, final_deadline,
					allowed_submission_types, status, created_by
				) VALUES (
					$1, $2, NULLIF($3,''), NULLIF($4,''), NULLIF($5,''),
					NULLIF($6,''), NULLIF($7,'{}'::text[]), $8::project_category, $9,
					(SELECT id FROM batches WHERE short_id = $10 AND deleted_at IS NULL),
					(SELECT id FROM modules WHERE short_id = NULLIF($11,'') AND deleted_at IS NULL),
					$12, NULLIF($13,'')::DATE, $14,
					$15, $16::project_status, $17
				)
				RETURNING *
			)
			%s`, insSelect),
			shortID, in.Title, in.ProblemStatement, in.Requirements, in.ExpectedDeliverables,
			in.EvaluationCriteria, in.ReferenceFiles, in.Category, in.IsTeamProject,
			in.BatchShortID, in.ModuleShortID, in.MaxMarks, in.StartDate, in.FinalDeadline,
			in.AllowedSubmissionTypes, status, createdBy,
		))
		if err == nil {
			return &p, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
				continue
			}
			if pgErr.Code == "23502" {
				return nil, fmt.Errorf("invalid batch_short_id: batch not found")
			}
		}
		return nil, fmt.Errorf("insert project: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

func (r *ProjectRepository) scanAll(ctx context.Context, q string, args ...interface{}) ([]models.Project, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *ProjectRepository) FindAll(ctx context.Context, f models.ProjectFilter) ([]models.Project, error) {
	where := []string{"p.deleted_at IS NULL"}
	args := []interface{}{}
	i := 1
	if f.BatchShortID != "" {
		where = append(where, fmt.Sprintf("b.short_id = $%d", i))
		args = append(args, f.BatchShortID)
		i++
	}
	if f.Status != "" {
		where = append(where, fmt.Sprintf("p.status = $%d::project_status", i))
		args = append(args, f.Status)
		i++
	}
	q := fmt.Sprintf("%s WHERE %s ORDER BY p.created_at DESC", projectBaseSelect, strings.Join(where, " AND "))
	return r.scanAll(ctx, q, args...)
}

func (r *ProjectRepository) FindAllForMentor(ctx context.Context, mentorID string) ([]models.Project, error) {
	q := fmt.Sprintf(`%s
		WHERE p.deleted_at IS NULL AND (b.batch_manager_id = $1 OR b.additional_manager_id = $1)
		ORDER BY p.created_at DESC`, projectBaseSelect)
	return r.scanAll(ctx, q, mentorID)
}

func (r *ProjectRepository) FindAllForStudent(ctx context.Context, studentID string) ([]models.Project, error) {
	q := fmt.Sprintf(`%s
		JOIN batch_students bs ON bs.batch_id = p.batch_id AND bs.user_id = $1
		WHERE p.deleted_at IS NULL AND p.status = 'active'
		ORDER BY p.final_deadline ASC`, projectBaseSelect)
	return r.scanAll(ctx, q, studentID)
}

// FindDueForDeadlineReminder returns active projects whose final deadline
// falls within the next 24 hours and haven't been reminded about yet.
func (r *ProjectRepository) FindDueForDeadlineReminder(ctx context.Context) ([]models.Project, error) {
	q := fmt.Sprintf(`%s
		WHERE p.deleted_at IS NULL AND p.status = 'active' AND p.deadline_reminder_sent = FALSE
		  AND p.final_deadline BETWEEN NOW() AND NOW() + INTERVAL '24 hours'
		ORDER BY p.final_deadline ASC`, projectBaseSelect)
	return r.scanAll(ctx, q)
}

// MarkDeadlineReminderSent flags a project so its deadline reminder fires once.
func (r *ProjectRepository) MarkDeadlineReminderSent(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE projects SET deadline_reminder_sent = TRUE WHERE id = $1::UUID`, id)
	return err
}

func (r *ProjectRepository) FindByShortID(ctx context.Context, shortID string) (*models.Project, error) {
	q := fmt.Sprintf("%s WHERE p.short_id = $1 AND p.deleted_at IS NULL", projectBaseSelect)
	p, err := scanProject(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}

func (r *ProjectRepository) Update(ctx context.Context, shortID string, in models.UpdateProjectInput) error {
	args := []interface{}{shortID}
	setClauses := []string{}
	i := 2

	add := func(clause string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf(clause, i))
		args = append(args, val)
		i++
	}

	if in.Title != nil {
		add("title = $%d", *in.Title)
	}
	if in.ProblemStatement != nil {
		add("problem_statement = NULLIF($%d,'')", *in.ProblemStatement)
	}
	if in.Requirements != nil {
		add("requirements = NULLIF($%d,'')", *in.Requirements)
	}
	if in.ExpectedDeliverables != nil {
		add("expected_deliverables = NULLIF($%d,'')", *in.ExpectedDeliverables)
	}
	if in.EvaluationCriteria != nil {
		add("evaluation_criteria = NULLIF($%d,'')", *in.EvaluationCriteria)
	}
	if in.ReferenceFiles != nil {
		add("reference_files = $%d", in.ReferenceFiles)
	}
	if in.Category != nil {
		add("category = $%d::project_category", *in.Category)
	}
	if in.IsTeamProject != nil {
		add("is_team_project = $%d", *in.IsTeamProject)
	}
	if in.BatchShortID != nil {
		add("batch_id = (SELECT id FROM batches WHERE short_id = $%d AND deleted_at IS NULL)", *in.BatchShortID)
	}
	if in.ModuleShortID != nil {
		add("module_id = (SELECT id FROM modules WHERE short_id = NULLIF($%d,'') AND deleted_at IS NULL)", *in.ModuleShortID)
	}
	if in.MaxMarks != nil {
		add("max_marks = $%d", *in.MaxMarks)
	}
	if in.StartDate != nil {
		add("start_date = NULLIF($%d,'')::DATE", *in.StartDate)
	}
	if in.FinalDeadline != nil {
		add("final_deadline = $%d", *in.FinalDeadline)
	}
	if in.AllowedSubmissionTypes != nil {
		add("allowed_submission_types = $%d", in.AllowedSubmissionTypes)
	}
	if in.Status != nil {
		add("status = $%d::project_status", *in.Status)
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}
	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf("UPDATE projects SET %s WHERE short_id = $1 AND deleted_at IS NULL", strings.Join(setClauses, ", "))
	result, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *ProjectRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx, `UPDATE projects SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ── Milestones ───────────────────────────────────────────────────────────────

const milestoneSelectCols = `id, short_id, project_id, title, COALESCE(description,''), due_date, order_index, is_final, created_at, updated_at`

func scanMilestone(row pgx.Row) (models.ProjectMilestone, error) {
	var m models.ProjectMilestone
	err := row.Scan(&m.ID, &m.ShortID, &m.ProjectID, &m.Title, &m.Description, &m.DueDate, &m.OrderIndex, &m.IsFinal, &m.CreatedAt, &m.UpdatedAt)
	return m, err
}

func (r *ProjectRepository) AddMilestone(ctx context.Context, projectShortID string, in models.CreateMilestoneInput) (*models.ProjectMilestone, error) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		m, err := scanMilestone(r.pool.QueryRow(ctx, fmt.Sprintf(`
			INSERT INTO project_milestones (short_id, project_id, title, description, due_date, order_index, is_final)
			SELECT $1, p.id, $2, NULLIF($3,''), $4, $5, $6
			FROM projects p WHERE p.short_id = $7 AND p.deleted_at IS NULL
			RETURNING %s`, milestoneSelectCols),
			shortID, in.Title, in.Description, in.DueDate, in.OrderIndex, in.IsFinal, projectShortID,
		))
		if err == nil {
			return &m, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return nil, fmt.Errorf("insert milestone: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

func (r *ProjectRepository) GetMilestones(ctx context.Context, projectShortID string) ([]models.ProjectMilestone, error) {
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT pm.id, pm.short_id, pm.project_id, pm.title, COALESCE(pm.description,''), pm.due_date, pm.order_index, pm.is_final, pm.created_at, pm.updated_at
		FROM project_milestones pm
		JOIN projects p ON pm.project_id = p.id
		WHERE p.short_id = $1 AND pm.deleted_at IS NULL
		ORDER BY pm.order_index ASC, pm.created_at ASC`),
		projectShortID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ProjectMilestone
	for rows.Next() {
		m, err := scanMilestone(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *ProjectRepository) UpdateMilestone(ctx context.Context, milestoneShortID string, in models.UpdateMilestoneInput) error {
	args := []interface{}{milestoneShortID}
	setClauses := []string{}
	i := 2
	add := func(clause string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf(clause, i))
		args = append(args, val)
		i++
	}
	if in.Title != nil {
		add("title = $%d", *in.Title)
	}
	if in.Description != nil {
		add("description = NULLIF($%d,'')", *in.Description)
	}
	if in.DueDate != nil {
		add("due_date = $%d", *in.DueDate)
	}
	if in.OrderIndex != nil {
		add("order_index = $%d", *in.OrderIndex)
	}
	if in.IsFinal != nil {
		add("is_final = $%d", *in.IsFinal)
	}
	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}
	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf("UPDATE project_milestones SET %s WHERE short_id = $1 AND deleted_at IS NULL", strings.Join(setClauses, ", "))
	result, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *ProjectRepository) DeleteMilestone(ctx context.Context, milestoneShortID string) error {
	result, err := r.pool.Exec(ctx, `UPDATE project_milestones SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, milestoneShortID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ── Teams ────────────────────────────────────────────────────────────────────

func (r *ProjectRepository) CreateTeam(ctx context.Context, projectShortID string, in models.CreateTeamInput, addedBy string) (*models.ProjectTeam, error) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		var teamID string
		err := r.pool.QueryRow(ctx, `
			INSERT INTO project_teams (short_id, project_id, name)
			SELECT $1, p.id, $2 FROM projects p WHERE p.short_id = $3 AND p.deleted_at IS NULL
			RETURNING id`,
			shortID, in.Name, projectShortID,
		).Scan(&teamID)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
				continue
			}
			return nil, fmt.Errorf("insert team: %w", err)
		}

		if len(in.StudentIDs) > 0 {
			if _, err := r.pool.Exec(ctx, `
				INSERT INTO project_team_members (team_id, user_id, added_by)
				SELECT $1, u.id, $2
				FROM unnest($3::uuid[]) AS uid(user_id)
				JOIN users u ON u.id = uid.user_id AND u.deleted_at IS NULL AND u.role = 'student'
				ON CONFLICT (team_id, user_id) DO NOTHING`,
				teamID, addedBy, in.StudentIDs,
			); err != nil {
				return nil, fmt.Errorf("add initial team members: %w", err)
			}
		}

		return r.getTeamByID(ctx, teamID)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

func (r *ProjectRepository) getTeamByID(ctx context.Context, teamID string) (*models.ProjectTeam, error) {
	var t models.ProjectTeam
	err := r.pool.QueryRow(ctx, `SELECT id, short_id, project_id, name, created_at FROM project_teams WHERE id = $1`, teamID).
		Scan(&t.ID, &t.ShortID, &t.ProjectID, &t.Name, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	members, err := r.getTeamMembers(ctx, teamID)
	if err != nil {
		return nil, err
	}
	t.Members = members
	return &t, nil
}

func (r *ProjectRepository) getTeamMembers(ctx context.Context, teamID string) ([]models.ProjectTeamMember, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.first_name, u.last_name, u.email, ptm.joined_at
		FROM project_team_members ptm
		JOIN users u ON u.id = ptm.user_id AND u.deleted_at IS NULL
		WHERE ptm.team_id = $1
		ORDER BY ptm.joined_at ASC`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ProjectTeamMember
	for rows.Next() {
		var m models.ProjectTeamMember
		if err := rows.Scan(&m.UserID, &m.FirstName, &m.LastName, &m.Email, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetTeams returns every team for a project, each with its members.
func (r *ProjectRepository) GetTeams(ctx context.Context, projectShortID string) ([]models.ProjectTeam, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT t.id, t.short_id, t.project_id, t.name, t.created_at
		FROM project_teams t
		JOIN projects p ON t.project_id = p.id
		WHERE p.short_id = $1 AND t.deleted_at IS NULL
		ORDER BY t.created_at ASC`,
		projectShortID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []models.ProjectTeam
	for rows.Next() {
		var t models.ProjectTeam
		if err := rows.Scan(&t.ID, &t.ShortID, &t.ProjectID, &t.Name, &t.CreatedAt); err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range teams {
		members, err := r.getTeamMembers(ctx, teams[i].ID)
		if err != nil {
			return nil, err
		}
		teams[i].Members = members
	}
	return teams, nil
}

func (r *ProjectRepository) DeleteTeam(ctx context.Context, teamShortID string) error {
	result, err := r.pool.Exec(ctx, `UPDATE project_teams SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, teamShortID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *ProjectRepository) AddTeamMembers(ctx context.Context, teamShortID string, studentIDs []string, addedBy string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO project_team_members (team_id, user_id, added_by)
		SELECT t.id, uid.user_id, $3
		FROM project_teams t
		CROSS JOIN unnest($2::uuid[]) AS uid(user_id)
		JOIN users u ON u.id = uid.user_id AND u.deleted_at IS NULL AND u.role = 'student'
		WHERE t.short_id = $1 AND t.deleted_at IS NULL
		ON CONFLICT (team_id, user_id) DO NOTHING`,
		teamShortID, studentIDs, addedBy,
	)
	return err
}

func (r *ProjectRepository) RemoveTeamMember(ctx context.Context, teamShortID, userID string) error {
	result, err := r.pool.Exec(ctx, `
		DELETE FROM project_team_members
		WHERE team_id = (SELECT id FROM project_teams WHERE short_id = $1 AND deleted_at IS NULL)
		  AND user_id = $2::uuid`,
		teamShortID, userID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// FindStudentTeam returns the short_id of the team the student belongs to for a
// given project, or "" if they're not on any team for it.
func (r *ProjectRepository) FindStudentTeam(ctx context.Context, projectShortID, studentID string) (string, error) {
	var teamShortID string
	err := r.pool.QueryRow(ctx, `
		SELECT t.short_id
		FROM project_teams t
		JOIN project_team_members ptm ON ptm.team_id = t.id
		JOIN projects p ON t.project_id = p.id
		WHERE p.short_id = $1 AND ptm.user_id = $2 AND t.deleted_at IS NULL`,
		projectShortID, studentID,
	).Scan(&teamShortID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return teamShortID, err
}

// ── Submissions ──────────────────────────────────────────────────────────────

const projectSubmissionSelect = `
	SELECT ps.id, ps.short_id, pm.short_id,
	       COALESCE(ps.student_id::TEXT, ''), COALESCE(CONCAT(su.first_name, ' ', su.last_name), ''),
	       COALESCE(t.short_id, ''), COALESCE(t.name, ''),
	       ps.submission_type::TEXT, COALESCE(ps.content,''), COALESCE(ps.file_url,''),
	       ps.status::TEXT, ps.marks, COALESCE(ps.feedback,''),
	       ps.submitted_at, ps.evaluated_at, COALESCE(ps.evaluated_by::TEXT,''),
	       ps.created_at, ps.updated_at
	FROM project_submissions ps
	JOIN project_milestones pm ON ps.milestone_id = pm.id
	LEFT JOIN users su ON ps.student_id = su.id AND su.deleted_at IS NULL
	LEFT JOIN project_teams t ON ps.team_id = t.id`

func scanProjectSubmission(row pgx.Row) (models.ProjectSubmission, error) {
	var s models.ProjectSubmission
	err := row.Scan(
		&s.ID, &s.ShortID, &s.MilestoneShortID,
		&s.StudentID, &s.StudentName,
		&s.TeamShortID, &s.TeamName,
		&s.SubmissionType, &s.Content, &s.FileURL,
		&s.Status, &s.Marks, &s.Feedback,
		&s.SubmittedAt, &s.EvaluatedAt, &s.EvaluatedBy,
		&s.CreatedAt, &s.UpdatedAt,
	)
	return s, err
}

// CreateOrResubmitSubmission inserts a submission for a milestone, keyed by either
// studentID or teamID (exactly one must be non-empty). Overwrites an existing
// submission only if it's in "resubmission_required" state.
func (r *ProjectRepository) CreateOrResubmitSubmission(ctx context.Context, milestoneShortID, studentID, teamID string, in models.CreateProjectSubmissionInput) (*models.ProjectSubmission, error) {
	var existingStatus string
	checkQ := `SELECT ps.status::TEXT FROM project_submissions ps JOIN project_milestones pm ON ps.milestone_id = pm.id WHERE pm.short_id = $1 AND `
	var err error
	if teamID != "" {
		err = r.pool.QueryRow(ctx, checkQ+"ps.team_id = $2", milestoneShortID, teamID).Scan(&existingStatus)
	} else {
		err = r.pool.QueryRow(ctx, checkQ+"ps.student_id = $2", milestoneShortID, studentID).Scan(&existingStatus)
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check existing submission: %w", err)
	}
	if err == nil && existingStatus != "resubmission_required" {
		return nil, fmt.Errorf("already submitted")
	}

	conflictCol := "student_id"
	identifierArg := studentID
	if teamID != "" {
		conflictCol = "team_id"
	}

	// See the comment on assessment_repo.go's Create for why this reads FROM
	// ins rather than FROM project_submissions.
	insSelect := strings.Replace(projectSubmissionSelect, "FROM project_submissions ps", "FROM ins ps", 1)

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		var identifierSelect string
		var identifierValue interface{}
		if teamID != "" {
			identifierSelect = "NULL, (SELECT id FROM project_teams WHERE short_id = $6 AND deleted_at IS NULL)"
			identifierValue = teamID
		} else {
			identifierSelect = "$6::UUID, NULL"
			identifierValue = identifierArg
		}

		s, err := scanProjectSubmission(r.pool.QueryRow(ctx, fmt.Sprintf(`
			WITH ins AS (
				INSERT INTO project_submissions (
					short_id, milestone_id, student_id, team_id, submission_type, content, file_url, status, submitted_at
				)
				SELECT $1, pm.id, %s, $2::project_submission_type, NULLIF($3,''), NULLIF($4,''),
				       (CASE WHEN pm.due_date IS NOT NULL AND NOW() > pm.due_date THEN 'late' ELSE 'submitted' END)::project_submission_status,
				       NOW()
				FROM project_milestones pm WHERE pm.short_id = $5
				ON CONFLICT (milestone_id, %s) DO UPDATE SET
					submission_type = EXCLUDED.submission_type,
					content = EXCLUDED.content,
					file_url = EXCLUDED.file_url,
					status = EXCLUDED.status,
					submitted_at = NOW(),
					marks = NULL, feedback = NULL, evaluated_at = NULL, evaluated_by = NULL,
					updated_at = NOW()
				RETURNING *
			)
			%s`, identifierSelect, conflictCol, insSelect),
			shortID, in.SubmissionType, in.Content, in.FileURL, milestoneShortID, identifierValue,
		))
		if err == nil {
			return &s, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return nil, fmt.Errorf("insert submission: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

func (r *ProjectRepository) FindMySubmission(ctx context.Context, milestoneShortID, studentID, teamID string) (*models.ProjectSubmission, error) {
	var q string
	var arg string
	if teamID != "" {
		q = projectSubmissionSelect + " WHERE pm.short_id = $1 AND ps.team_id = (SELECT id FROM project_teams WHERE short_id = $2)"
		arg = teamID
	} else {
		q = projectSubmissionSelect + " WHERE pm.short_id = $1 AND ps.student_id = $2"
		arg = studentID
	}
	s, err := scanProjectSubmission(r.pool.QueryRow(ctx, q, milestoneShortID, arg))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &s, err
}

func (r *ProjectRepository) FindAllSubmissions(ctx context.Context, milestoneShortID string) ([]models.ProjectSubmission, error) {
	q := projectSubmissionSelect + " WHERE pm.short_id = $1 ORDER BY ps.submitted_at DESC"
	rows, err := r.pool.Query(ctx, q, milestoneShortID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ProjectSubmission
	for rows.Next() {
		s, err := scanProjectSubmission(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// FindAllSubmissionsForMentor returns every project-milestone submission
// across every project — or, when mentorID is non-empty, only those in
// batches that mentor manages — newest first. This is the cross-project feed
// behind the unified Submissions workspace.
func (r *ProjectRepository) FindAllSubmissionsForMentor(ctx context.Context, mentorID string) ([]models.ProjectSubmission, error) {
	q := `
		SELECT ps.id, ps.short_id, pm.short_id, p.title, pm.title, b.short_id, b.batch_number,
		       COALESCE(ps.student_id::TEXT, ''), COALESCE(CONCAT(su.first_name, ' ', su.last_name), ''),
		       COALESCE(t.short_id, ''), COALESCE(t.name, ''),
		       ps.submission_type::TEXT, COALESCE(ps.content,''), COALESCE(ps.file_url,''),
		       ps.status::TEXT, ps.marks, COALESCE(ps.feedback,''),
		       ps.submitted_at, ps.evaluated_at, COALESCE(ps.evaluated_by::TEXT,''),
		       ps.created_at, ps.updated_at
		FROM project_submissions ps
		JOIN project_milestones pm ON ps.milestone_id = pm.id
		JOIN projects p ON pm.project_id = p.id
		JOIN batches  b ON p.batch_id     = b.id
		LEFT JOIN users su ON ps.student_id = su.id AND su.deleted_at IS NULL
		LEFT JOIN project_teams t ON ps.team_id = t.id`
	args := []interface{}{}
	if mentorID != "" {
		q += ` WHERE b.batch_manager_id = $1 OR b.additional_manager_id = $1`
		args = append(args, mentorID)
	}
	q += ` ORDER BY ps.submitted_at DESC LIMIT 500`

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ProjectSubmission
	for rows.Next() {
		var s models.ProjectSubmission
		if err := rows.Scan(
			&s.ID, &s.ShortID, &s.MilestoneShortID, &s.ProjectTitle, &s.MilestoneTitle, &s.BatchShortID, &s.BatchNumber,
			&s.StudentID, &s.StudentName,
			&s.TeamShortID, &s.TeamName,
			&s.SubmissionType, &s.Content, &s.FileURL,
			&s.Status, &s.Marks, &s.Feedback,
			&s.SubmittedAt, &s.EvaluatedAt, &s.EvaluatedBy,
			&s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *ProjectRepository) GradeSubmission(ctx context.Context, milestoneShortID, submissionShortID, evaluatedBy string, in models.GradeProjectSubmissionInput) error {
	status := in.Status
	if status == "" {
		status = "evaluated"
	}
	result, err := r.pool.Exec(ctx, `
		UPDATE project_submissions SET
			marks = $1, feedback = NULLIF($2,''), status = $3::project_submission_status,
			evaluated_at = NOW(), evaluated_by = $4, updated_at = NOW()
		WHERE short_id = $5
		  AND milestone_id = (SELECT id FROM project_milestones WHERE short_id = $6)`,
		in.Marks, in.Feedback, status, evaluatedBy, submissionShortID, milestoneShortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
