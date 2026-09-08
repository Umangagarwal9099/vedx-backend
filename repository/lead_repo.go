package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type LeadRepository struct {
	pool *pgxpool.Pool
}

func NewLeadRepository(pool *pgxpool.Pool) *LeadRepository {
	return &LeadRepository{pool: pool}
}

const leadBaseSelect = `
	SELECT l.id, l.short_id, COALESCE(l.college_id::TEXT, ''), l.name, l.phone, COALESCE(l.email, ''), COALESCE(l.city, ''),
	       l.course_interest, COALESCE(c.short_id, ''),
	       l.source::TEXT, l.status::TEXT, l.priority::TEXT,
	       COALESCE(l.assigned_to::TEXT, ''), COALESCE(CONCAT(au.first_name, ' ', au.last_name), ''),
	       l.assigned_at, COALESCE(l.assigned_by::TEXT, ''),
	       l.next_follow_up_at, l.last_contacted_at, l.converted_at,
	       COALESCE(l.notes, ''),
	       l.created_by, CONCAT(cu.first_name, ' ', cu.last_name),
	       l.created_at, l.updated_at
	FROM leads l
	JOIN users cu ON l.created_by = cu.id AND cu.deleted_at IS NULL
	LEFT JOIN users au ON l.assigned_to = au.id AND au.deleted_at IS NULL
	LEFT JOIN courses c ON l.course_id = c.id AND c.deleted_at IS NULL`

func scanLead(row pgx.Row) (models.Lead, error) {
	var l models.Lead
	err := row.Scan(
		&l.ID, &l.ShortID, &l.CollegeID, &l.Name, &l.Phone, &l.Email, &l.City,
		&l.CourseInterest, &l.CourseShortID,
		&l.Source, &l.Status, &l.Priority,
		&l.AssignedTo, &l.AssignedToName,
		&l.AssignedAt, &l.AssignedBy,
		&l.NextFollowUpAt, &l.LastContactedAt, &l.ConvertedAt,
		&l.Notes,
		&l.CreatedBy, &l.CreatedByName,
		&l.CreatedAt, &l.UpdatedAt,
	)
	return l, err
}

func (r *LeadRepository) scanAll(ctx context.Context, q string, args ...interface{}) ([]models.Lead, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Lead
	for rows.Next() {
		l, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Create inserts a lead, best-effort resolving course_id by exact name match
// against the courses table (leaves it NULL if nothing matches). collegeID
// must never be empty — resolved by the caller (see
// controller.resolveTargetCollege) so no newly created lead is ever left
// with a NULL college_id.
func (r *LeadRepository) Create(ctx context.Context, in models.CreateLeadInput, createdBy, collegeID string) (*models.Lead, error) {
	source := in.Source
	if source == "" {
		source = "manual"
	}
	priority := in.Priority
	if priority == "" {
		priority = "medium"
	}

	// See the comment on assessment_repo.go's Create for why this reads FROM
	// ins rather than FROM leads.
	insSelect := strings.Replace(leadBaseSelect, "FROM leads l", "FROM ins l", 1)

	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		l, err := scanLead(r.pool.QueryRow(ctx, fmt.Sprintf(`
			WITH ins AS (
				INSERT INTO leads (
					short_id, college_id, name, phone, email, city, course_interest, course_id,
					source, status, priority, notes, created_by
				) VALUES (
					$1, $2::UUID, $3, $4, NULLIF($5,''), NULLIF($6,''), $7,
					(SELECT id FROM courses WHERE LOWER(name) = LOWER($7) AND deleted_at IS NULL LIMIT 1),
					$8::lead_source, 'new'::lead_status, $9::lead_priority, NULLIF($10,''), $11
				)
				RETURNING *
			)
			%s`, insSelect),
			shortID, collegeID, in.Name, in.Phone, in.Email, in.City, in.CourseInterest,
			source, priority, in.Notes, createdBy,
		))
		if err == nil {
			return &l, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "short_id") {
			continue
		}
		return nil, fmt.Errorf("insert lead: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// ExistsRecentDuplicate reports whether a non-deleted lead with the same
// email or phone was created within the last `within` window — used by the
// public website-intake endpoint to collapse accidental resubmissions
// (double-click, browser retry, the same visitor filling two forms) into the
// one lead, without blocking a genuine fresh enquiry weeks later. An empty
// email/phone is ignored (never matches).
func (r *LeadRepository) ExistsRecentDuplicate(ctx context.Context, email, phone string, within time.Duration) (bool, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	phone = strings.TrimSpace(phone)
	if email == "" && phone == "" {
		return false, nil
	}
	since := time.Now().Add(-within)
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM leads
			WHERE deleted_at IS NULL
			  AND created_at >= $1
			  AND (
			        ($2 <> '' AND LOWER(email) = $2)
			     OR ($3 <> '' AND phone = $3)
			  )
		)`, since, email, phone).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check recent duplicate lead: %w", err)
	}
	return exists, nil
}

// buildLeadWhere returns the shared WHERE clauses + args for both the
// unscoped (admin) and employee-scoped views, given the filter and an
// optional employeeID (non-empty pins results to that employee's leads) and
// collegeID (non-empty scopes to that college for non-super-admin callers).
func buildLeadWhere(f models.LeadFilter, employeeID, collegeID string) ([]string, []interface{}) {
	where := []string{"l.deleted_at IS NULL"}
	args := []interface{}{}
	i := 1

	if collegeID != "" {
		where = append(where, fmt.Sprintf("l.college_id = $%d::UUID", i))
		args = append(args, collegeID)
		i++
	}
	if employeeID != "" {
		where = append(where, fmt.Sprintf("l.assigned_to = $%d", i))
		args = append(args, employeeID)
		i++
	}
	if f.Status != "" {
		where = append(where, fmt.Sprintf("l.status = $%d::lead_status", i))
		args = append(args, f.Status)
		i++
	}
	if f.Priority != "" {
		where = append(where, fmt.Sprintf("l.priority = $%d::lead_priority", i))
		args = append(args, f.Priority)
		i++
	}
	if f.Course != "" {
		where = append(where, fmt.Sprintf("l.course_interest ILIKE $%d", i))
		args = append(args, "%"+f.Course+"%")
		i++
	}
	if f.City != "" {
		where = append(where, fmt.Sprintf("l.city ILIKE $%d", i))
		args = append(args, "%"+f.City+"%")
		i++
	}
	if f.EmployeeID != "" && employeeID == "" {
		where = append(where, fmt.Sprintf("l.assigned_to = $%d", i))
		args = append(args, f.EmployeeID)
		i++
	}
	if f.Today {
		where = append(where, "l.assigned_at::DATE = CURRENT_DATE")
	}
	if f.FollowUpDue {
		where = append(where, "l.next_follow_up_at::DATE = CURRENT_DATE")
	}
	if f.Overdue {
		where = append(where, "l.next_follow_up_at < NOW() AND l.status NOT IN ('converted', 'lost', 'not_interested')")
	}
	if f.DateFrom != "" {
		where = append(where, fmt.Sprintf("l.created_at >= $%d::DATE", i))
		args = append(args, f.DateFrom)
		i++
	}
	if f.DateTo != "" {
		where = append(where, fmt.Sprintf("l.created_at < ($%d::DATE + INTERVAL '1 day')", i))
		args = append(args, f.DateTo)
		i++
	}
	return where, args
}

// FindAll returns every non-deleted lead (staff view), filtered. collegeID
// scopes results for non-super-admin callers (empty = unscoped).
func (r *LeadRepository) FindAll(ctx context.Context, f models.LeadFilter, collegeID string) ([]models.Lead, error) {
	where, args := buildLeadWhere(f, "", collegeID)
	q := fmt.Sprintf("%s WHERE %s ORDER BY l.created_at DESC", leadBaseSelect, strings.Join(where, " AND "))
	return r.scanAll(ctx, q, args...)
}

// FindAllForEmployee returns only leads assigned to the given employee (or
// team_lead carrying a personal quota), filtered. collegeID scopes further
// for non-super-admin callers (empty = unscoped).
func (r *LeadRepository) FindAllForEmployee(ctx context.Context, employeeID string, f models.LeadFilter, collegeID string) ([]models.Lead, error) {
	where, args := buildLeadWhere(f, employeeID, collegeID)
	q := fmt.Sprintf("%s WHERE %s ORDER BY l.next_follow_up_at ASC NULLS LAST, l.created_at DESC", leadBaseSelect, strings.Join(where, " AND "))
	return r.scanAll(ctx, q, args...)
}

func (r *LeadRepository) FindByShortID(ctx context.Context, shortID string) (*models.Lead, error) {
	q := fmt.Sprintf("%s WHERE l.short_id = $1 AND l.deleted_at IS NULL", leadBaseSelect)
	l, err := scanLead(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &l, err
}

// Update applies a partial update. When status is set to 'converted', also
// stamps converted_at; changing away from 'converted' clears it.
func (r *LeadRepository) Update(ctx context.Context, shortID string, in models.UpdateLeadInput) error {
	args := []interface{}{shortID}
	setClauses := []string{}
	i := 2

	add := func(clause string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf(clause, i))
		args = append(args, val)
		i++
	}

	if in.Name != nil {
		add("name = $%d", *in.Name)
	}
	if in.Phone != nil {
		add("phone = $%d", *in.Phone)
	}
	if in.Email != nil {
		add("email = NULLIF($%d,'')", *in.Email)
	}
	if in.City != nil {
		add("city = NULLIF($%d,'')", *in.City)
	}
	if in.CourseInterest != nil {
		add("course_interest = $%d", *in.CourseInterest)
	}
	if in.Status != nil {
		add("status = $%d::lead_status", *in.Status)
		if *in.Status == "converted" {
			setClauses = append(setClauses, "converted_at = NOW()")
		} else {
			setClauses = append(setClauses, "converted_at = NULL")
		}
	}
	if in.Priority != nil {
		add("priority = $%d::lead_priority", *in.Priority)
	}
	if in.NextFollowUpAt != nil {
		add("next_follow_up_at = $%d", *in.NextFollowUpAt)
		setClauses = append(setClauses, "follow_up_reminder_sent = FALSE")
	}
	if in.Notes != nil {
		add("notes = NULLIF($%d,'')", *in.Notes)
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}
	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf("UPDATE leads SET %s WHERE short_id = $1 AND deleted_at IS NULL", strings.Join(setClauses, ", "))
	result, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// RecordContact updates a lead's status/next_follow_up_at and stamps
// last_contacted_at = NOW(), in one statement — called right after a call log
// is written, so a lead's current state always reflects its latest call.
func (r *LeadRepository) RecordContact(ctx context.Context, shortID, status string, nextFollowUpAt *time.Time) error {
	convertedAtClause := "converted_at = NULL"
	if status == "converted" {
		convertedAtClause = "converted_at = NOW()"
	}
	q := fmt.Sprintf(`
		UPDATE leads SET
			status = $2::lead_status,
			next_follow_up_at = $3,
			follow_up_reminder_sent = FALSE,
			last_contacted_at = NOW(),
			%s,
			updated_at = NOW()
		WHERE short_id = $1 AND deleted_at IS NULL`, convertedAtClause)

	result, err := r.pool.Exec(ctx, q, shortID, status, nextFollowUpAt)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *LeadRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx,
		`UPDATE leads SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// BulkImport inserts every valid parsed row, best-effort — a row that fails
// (e.g. missing name/phone) is recorded in the result's Skipped list rather
// than aborting the whole import. Every imported row lands under collegeID —
// for a College Admin caller that's always forced to their own college; for
// super_admin the caller resolves it explicitly beforehand (or defaults it
// to the Internal EdTech Platform), matching Create's contract.
func (r *LeadRepository) BulkImport(ctx context.Context, rows []models.LeadImportRow, createdBy, collegeID string) (*models.LeadImportResult, error) {
	result := &models.LeadImportResult{Skipped: []models.LeadImportRowError{}}

	for _, row := range rows {
		if strings.TrimSpace(row.Name) == "" || strings.TrimSpace(row.Phone) == "" {
			result.Skipped = append(result.Skipped, models.LeadImportRowError{
				RowNumber: row.RowNumber, Reason: "missing name or phone",
			})
			continue
		}

		_, err := r.Create(ctx, models.CreateLeadInput{
			Name:           row.Name,
			Phone:          row.Phone,
			Email:          row.Email,
			City:           row.City,
			CourseInterest: row.CourseInterest,
			Source:         "excel_import",
		}, createdBy, collegeID)
		if err != nil {
			result.Skipped = append(result.Skipped, models.LeadImportRowError{
				RowNumber: row.RowNumber, Reason: err.Error(),
			})
			continue
		}
		result.Imported++
	}

	return result, nil
}

// GetDashboard aggregates the daily-work cards for one employee (or a
// team_lead's own personal lead quota).
func (r *LeadRepository) GetDashboard(ctx context.Context, employeeID string) (*models.LeadDashboard, error) {
	var d models.LeadDashboard

	err := r.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE assigned_at::DATE = CURRENT_DATE) AS assigned_today,
			COUNT(*) FILTER (WHERE status NOT IN ('converted', 'lost', 'not_interested')) AS active,
			COUNT(*) FILTER (WHERE next_follow_up_at::DATE = CURRENT_DATE
				AND status NOT IN ('converted', 'lost', 'not_interested')) AS calls_to_make_today,
			COUNT(*) FILTER (WHERE next_follow_up_at < NOW()
				AND status NOT IN ('converted', 'lost', 'not_interested')) AS overdue,
			COUNT(*) FILTER (WHERE status = 'new') AS new_leads,
			COUNT(*) FILTER (WHERE status = 'interested') AS interested,
			COUNT(*) FILTER (WHERE status = 'converted') AS converted,
			COUNT(*) FILTER (WHERE status = 'not_reachable') AS not_reachable
		FROM leads
		WHERE assigned_to = $1 AND deleted_at IS NULL`,
		employeeID,
	).Scan(
		&d.LeadsAssignedToday, &d.TotalActiveLeads, &d.FollowUpsDueToday, &d.OverdueFollowUps,
		&d.NewLeads, &d.InterestedLeads, &d.ConvertedLeads, &d.NotReachableLeads,
	)
	if err != nil {
		return nil, fmt.Errorf("lead dashboard aggregate: %w", err)
	}

	err = r.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE called_at::DATE = CURRENT_DATE) AS calls_completed_today
		FROM lead_call_logs
		WHERE employee_id = $1`,
		employeeID,
	).Scan(&d.CallsCompletedToday)
	if err != nil {
		return nil, fmt.Errorf("lead dashboard call-log aggregate: %w", err)
	}

	d.CallsToMakeToday = d.FollowUpsDueToday
	d.PendingCalls = d.TotalActiveLeads - d.CallsCompletedToday
	if d.PendingCalls < 0 {
		d.PendingCalls = 0
	}

	return &d, nil
}

// CountConvertedToday returns how many of an employee's leads converted
// today — powers the daily work report's prefilled admissions suggestion.
func (r *LeadRepository) CountConvertedToday(ctx context.Context, employeeID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM leads
		WHERE assigned_to = $1 AND status = 'converted' AND deleted_at IS NULL
		  AND converted_at::DATE = CURRENT_DATE`,
		employeeID,
	).Scan(&count)
	return count, err
}

// FindDueForFollowUpReminder returns assigned, still-open leads whose
// next_follow_up_at has arrived (or passed) and haven't been reminded about
// yet — powers scheduler.RunLeadFollowUpReminders.
func (r *LeadRepository) FindDueForFollowUpReminder(ctx context.Context) ([]models.Lead, error) {
	q := fmt.Sprintf(`%s
		WHERE l.deleted_at IS NULL AND l.follow_up_reminder_sent = FALSE
		  AND l.next_follow_up_at <= NOW() AND l.assigned_to IS NOT NULL
		  AND l.status NOT IN ('converted', 'lost', 'not_interested')
		ORDER BY l.next_follow_up_at ASC`, leadBaseSelect)
	return r.scanAll(ctx, q)
}

// MarkFollowUpReminderSent flags a lead so its follow-up reminder fires once
// per due date (reset back to FALSE whenever next_follow_up_at changes).
func (r *LeadRepository) MarkFollowUpReminderSent(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE leads SET follow_up_reminder_sent = TRUE WHERE id = $1`, id)
	return err
}

// assignLeadsTx assigns each lead in leadShortIDs to employeeID, recording an
// assignment-history row per lead (action is "assign" or "reassign" — the
// caller decides which, since both are the same underlying operation).
func (r *LeadRepository) assignLeadsTx(ctx context.Context, leadShortIDs []string, employeeID, action, priority string, followUpAt *time.Time, performedBy string) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	assigned := 0
	for _, shortID := range leadShortIDs {
		var leadID string
		var oldAssigned *string
		err := tx.QueryRow(ctx,
			`SELECT id, assigned_to::TEXT FROM leads WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
		).Scan(&leadID, &oldAssigned)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return assigned, fmt.Errorf("lookup lead %s: %w", shortID, err)
		}

		setClauses := []string{"assigned_to = $2", "assigned_at = NOW()", "assigned_by = $3", "updated_at = NOW()"}
		args := []interface{}{shortID, employeeID, performedBy}
		i := 4
		if priority != "" {
			setClauses = append(setClauses, fmt.Sprintf("priority = $%d::lead_priority", i))
			args = append(args, priority)
			i++
		}
		if followUpAt != nil {
			setClauses = append(setClauses, fmt.Sprintf("next_follow_up_at = $%d", i))
			args = append(args, *followUpAt)
			i++
			setClauses = append(setClauses, "follow_up_reminder_sent = FALSE")
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf("UPDATE leads SET %s WHERE short_id = $1", strings.Join(setClauses, ", ")), args...); err != nil {
			return assigned, fmt.Errorf("assign lead %s: %w", shortID, err)
		}

		from := ""
		if oldAssigned != nil {
			from = *oldAssigned
		}
		historyShortID := util.GenerateShortID()
		if _, err := tx.Exec(ctx, `
			INSERT INTO lead_assignment_history
			  (short_id, lead_id, action, from_employee_id, to_employee_id, priority_set, follow_up_set_at, performed_by)
			VALUES ($1, $2, $3::lead_assignment_action, NULLIF($4,'')::UUID, $5, NULLIF($6,'')::lead_priority, $7, $8)`,
			historyShortID, leadID, action, from, employeeID, priority, followUpAt, performedBy,
		); err != nil {
			return assigned, fmt.Errorf("log assignment history for lead %s: %w", shortID, err)
		}

		assigned++
	}

	return assigned, tx.Commit(ctx)
}

// AssignBulk assigns one or many leads (currently unassigned or not) to an
// employee, with an optional priority/first-follow-up-date.
func (r *LeadRepository) AssignBulk(ctx context.Context, leadShortIDs []string, employeeID, priority string, followUpAt *time.Time, performedBy string) (int, error) {
	return r.assignLeadsTx(ctx, leadShortIDs, employeeID, "assign", priority, followUpAt, performedBy)
}

// Reassign moves already-assigned leads to a different employee.
func (r *LeadRepository) Reassign(ctx context.Context, leadShortIDs []string, employeeID, priority string, followUpAt *time.Time, performedBy string) (int, error) {
	return r.assignLeadsTx(ctx, leadShortIDs, employeeID, "reassign", priority, followUpAt, performedBy)
}

// Unassign clears assignment on the given leads.
func (r *LeadRepository) Unassign(ctx context.Context, leadShortIDs []string, performedBy string) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	unassigned := 0
	for _, shortID := range leadShortIDs {
		var leadID string
		var oldAssigned *string
		err := tx.QueryRow(ctx,
			`SELECT id, assigned_to::TEXT FROM leads WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
		).Scan(&leadID, &oldAssigned)
		if errors.Is(err, pgx.ErrNoRows) || oldAssigned == nil {
			continue
		}
		if err != nil {
			return unassigned, fmt.Errorf("lookup lead %s: %w", shortID, err)
		}

		if _, err := tx.Exec(ctx, `
			UPDATE leads SET assigned_to = NULL, assigned_at = NULL, assigned_by = NULL, updated_at = NOW()
			WHERE short_id = $1`, shortID,
		); err != nil {
			return unassigned, fmt.Errorf("unassign lead %s: %w", shortID, err)
		}

		historyShortID := util.GenerateShortID()
		if _, err := tx.Exec(ctx, `
			INSERT INTO lead_assignment_history (short_id, lead_id, action, from_employee_id, performed_by)
			VALUES ($1, $2, 'unassign', $3, $4)`,
			historyShortID, leadID, *oldAssigned, performedBy,
		); err != nil {
			return unassigned, fmt.Errorf("log unassign history for lead %s: %w", shortID, err)
		}

		unassigned++
	}

	return unassigned, tx.Commit(ctx)
}

// AutoAssignRoundRobin assigns each lead in leadShortIDs to whichever
// eligible employee (role employee/team_lead) currently has the fewest
// active (non-closed) leads, recomputed after each assignment so a large
// batch is spread evenly rather than dumped on whoever was least-loaded at
// the start.
func (r *LeadRepository) AutoAssignRoundRobin(ctx context.Context, leadShortIDs []string, performedBy string) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	assigned := 0
	for _, shortID := range leadShortIDs {
		var employeeID string
		// Eligible for a general lead: team_lead (oversight role, unchanged),
		// or an employee specifically in the Operations department — plain
		// employees outside Operations (HR, Digital Marketing, Manager, or no
		// department set) are no longer in the auto-assign pool.
		err := tx.QueryRow(ctx, `
			SELECT u.id
			FROM users u
			LEFT JOIN employees e ON e.user_id = u.id
			LEFT JOIN leads l ON l.assigned_to = u.id AND l.deleted_at IS NULL
				AND l.status NOT IN ('converted', 'lost', 'not_interested')
			WHERE (u.role = 'team_lead' OR (u.role = 'employee' AND e.department = 'operations'))
			  AND u.deleted_at IS NULL AND u.is_active = TRUE
			GROUP BY u.id
			ORDER BY COUNT(l.id) ASC, u.id ASC
			LIMIT 1`,
		).Scan(&employeeID)
		if errors.Is(err, pgx.ErrNoRows) {
			return assigned, fmt.Errorf("no eligible employees to auto-assign to")
		}
		if err != nil {
			return assigned, fmt.Errorf("pick least-loaded employee: %w", err)
		}

		var leadID string
		var oldAssigned *string
		err = tx.QueryRow(ctx,
			`SELECT id, assigned_to::TEXT FROM leads WHERE short_id = $1 AND deleted_at IS NULL`, shortID,
		).Scan(&leadID, &oldAssigned)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return assigned, fmt.Errorf("lookup lead %s: %w", shortID, err)
		}

		if _, err := tx.Exec(ctx, `
			UPDATE leads SET assigned_to = $2, assigned_at = NOW(), assigned_by = $3, updated_at = NOW()
			WHERE short_id = $1`, shortID, employeeID, performedBy,
		); err != nil {
			return assigned, fmt.Errorf("assign lead %s: %w", shortID, err)
		}

		from := ""
		if oldAssigned != nil {
			from = *oldAssigned
		}
		historyShortID := util.GenerateShortID()
		if _, err := tx.Exec(ctx, `
			INSERT INTO lead_assignment_history (short_id, lead_id, action, from_employee_id, to_employee_id, performed_by)
			VALUES ($1, $2, 'auto_assign', NULLIF($3,'')::UUID, $4, $5)`,
			historyShortID, leadID, from, employeeID, performedBy,
		); err != nil {
			return assigned, fmt.Errorf("log auto-assign history for lead %s: %w", shortID, err)
		}

		assigned++
	}

	return assigned, tx.Commit(ctx)
}

// GetTeamSummary returns one row per employee with any assigned lead —
// powers the admin's team-monitoring dashboard.
func (r *LeadRepository) GetTeamSummary(ctx context.Context) ([]models.EmployeeLeadSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.id, CONCAT(u.first_name, ' ', u.last_name),
		       COUNT(l.id) FILTER (WHERE l.status NOT IN ('converted', 'lost', 'not_interested')) AS active_leads,
		       COUNT(cl.id) FILTER (WHERE cl.called_at::DATE = CURRENT_DATE) AS calls_today,
		       COUNT(l.id) FILTER (WHERE l.status = 'converted') AS conversions
		FROM users u
		LEFT JOIN employees e ON e.user_id = u.id
		LEFT JOIN leads l ON l.assigned_to = u.id AND l.deleted_at IS NULL
		LEFT JOIN lead_call_logs cl ON cl.employee_id = u.id
		WHERE (u.role = 'team_lead' OR (u.role = 'employee' AND e.department = 'operations')) AND u.deleted_at IS NULL
		GROUP BY u.id, u.first_name, u.last_name
		ORDER BY u.first_name, u.last_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.EmployeeLeadSummary
	for rows.Next() {
		var s models.EmployeeLeadSummary
		if err := rows.Scan(&s.EmployeeID, &s.EmployeeName, &s.ActiveLeads, &s.CallsToday, &s.Conversions); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
