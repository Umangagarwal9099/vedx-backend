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

type HelpSupportRepository struct {
	pool *pgxpool.Pool
}

func NewHelpSupportRepository(pool *pgxpool.Pool) *HelpSupportRepository {
	return &HelpSupportRepository{pool: pool}
}

const faqSelectCols = `id, short_id, question, answer, category, order_index, is_published, created_at, updated_at`

func scanFAQ(row pgx.Row) (models.FAQ, error) {
	var f models.FAQ
	err := row.Scan(&f.ID, &f.ShortID, &f.Question, &f.Answer, &f.Category, &f.OrderIndex, &f.IsPublished, &f.CreatedAt, &f.UpdatedAt)
	return f, err
}

// ── FAQs ─────────────────────────────────────────────────────────────────────

func (r *HelpSupportRepository) CreateFAQ(ctx context.Context, in models.CreateFAQInput, createdBy string) (*models.FAQ, error) {
	published := true
	if in.IsPublished != nil {
		published = *in.IsPublished
	}
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		f, err := scanFAQ(r.pool.QueryRow(ctx, fmt.Sprintf(`
			INSERT INTO platform_faqs (short_id, question, answer, category, order_index, is_published, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7::UUID)
			RETURNING %s`, faqSelectCols),
			shortID, in.Question, in.Answer, in.Category, in.OrderIndex, published, createdBy,
		))
		if err == nil {
			return &f, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("insert faq: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

// FindAllPublished returns only published FAQs, ordered for display — the
// student-facing Help & Support page.
func (r *HelpSupportRepository) FindAllPublished(ctx context.Context) ([]models.FAQ, error) {
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM platform_faqs WHERE is_published = TRUE
		ORDER BY order_index ASC, created_at ASC`, faqSelectCols))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.FAQ
	for rows.Next() {
		f, err := scanFAQ(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FindAllAdmin returns every FAQ (published or not) for the admin management view.
func (r *HelpSupportRepository) FindAllAdmin(ctx context.Context) ([]models.FAQ, error) {
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM platform_faqs ORDER BY order_index ASC, created_at ASC`, faqSelectCols))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.FAQ
	for rows.Next() {
		f, err := scanFAQ(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r *HelpSupportRepository) UpdateFAQ(ctx context.Context, shortID string, in models.UpdateFAQInput) error {
	args := []interface{}{shortID}
	setClauses := []string{}
	i := 2

	add := func(clause string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf(clause, i))
		args = append(args, val)
		i++
	}
	if in.Question != nil {
		add("question = $%d", *in.Question)
	}
	if in.Answer != nil {
		add("answer = $%d", *in.Answer)
	}
	if in.Category != nil {
		add("category = $%d", *in.Category)
	}
	if in.OrderIndex != nil {
		add("order_index = $%d", *in.OrderIndex)
	}
	if in.IsPublished != nil {
		add("is_published = $%d", *in.IsPublished)
	}
	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}
	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf("UPDATE platform_faqs SET %s WHERE short_id = $1", strings.Join(setClauses, ", "))
	tag, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *HelpSupportRepository) DeleteFAQ(ctx context.Context, shortID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM platform_faqs WHERE short_id = $1`, shortID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ── Support tickets ──────────────────────────────────────────────────────────

const ticketSelectCols = `id, short_id, user_id, name, email, phone, subject, message, status::TEXT, resolved_at, created_at, updated_at`
const adminTicketSelectCols = `t.id, t.short_id, t.user_id, t.name, t.email, t.phone, t.subject, t.message, t.status::TEXT, t.resolved_at, t.created_at, t.updated_at`

func scanTicket(row pgx.Row) (models.SupportTicket, error) {
	var t models.SupportTicket
	err := row.Scan(&t.ID, &t.ShortID, &t.UserID, &t.Name, &t.Email, &t.Phone, &t.Subject, &t.Message, &t.Status, &t.ResolvedAt, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (r *HelpSupportRepository) CreateTicket(ctx context.Context, userID string, in models.CreateSupportTicketInput) (*models.SupportTicket, error) {
	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()
		t, err := scanTicket(r.pool.QueryRow(ctx, fmt.Sprintf(`
			INSERT INTO support_tickets (short_id, user_id, name, email, phone, subject, message)
			VALUES ($1, $2::UUID, $3, $4, $5, $6, $7)
			RETURNING %s`, ticketSelectCols),
			shortID, userID, in.Name, in.Email, in.Phone, in.Subject, in.Message,
		))
		if err == nil {
			return &t, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		return nil, fmt.Errorf("insert support ticket: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

func (r *HelpSupportRepository) FindMyTickets(ctx context.Context, userID string) ([]models.SupportTicket, error) {
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM support_tickets t
		WHERE t.user_id = $1::UUID
		ORDER BY t.created_at DESC`, ticketSelectCols),
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SupportTicket
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// FindAllForAdmin lists every ticket, joined with the submitting student's
// name for display — collegeID scopes it to that college's own students
// (same repository.CollegeFilter convention used everywhere else); empty
// means unscoped (super_admin).
func (r *HelpSupportRepository) FindAllForAdmin(ctx context.Context, collegeID string) ([]models.SupportTicket, error) {
	args := []interface{}{}
	collegeClause := ""
	if collegeID != "" {
		collegeClause = " AND u.college_id = $1::UUID"
		args = append(args, collegeID)
	}
	q := fmt.Sprintf(`
		SELECT %s, CONCAT(u.first_name, ' ', u.last_name)
		FROM support_tickets t
		JOIN users u ON u.id = t.user_id
		WHERE TRUE%s
		ORDER BY t.created_at DESC`, adminTicketSelectCols, collegeClause)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SupportTicket
	for rows.Next() {
		var t models.SupportTicket
		if err := rows.Scan(&t.ID, &t.ShortID, &t.UserID, &t.Name, &t.Email, &t.Phone, &t.Subject, &t.Message, &t.Status, &t.ResolvedAt, &t.CreatedAt, &t.UpdatedAt, &t.StudentName); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpdateStatus updates a ticket's status. collegeID scopes it the same way
// as FindAllForAdmin — a college_admin/college_staff caller can only move
// the status of a ticket submitted by one of their own college's users;
// empty collegeID (super_admin/staff) is unscoped. Without this, a
// college-scoped caller who merely knew/guessed another college's ticket
// short_id could resolve or reopen it.
func (r *HelpSupportRepository) UpdateStatus(ctx context.Context, shortID string, status models.SupportTicketStatus, collegeID string) error {
	resolvedAtClause := "resolved_at = NULL"
	if status == models.SupportTicketResolved {
		resolvedAtClause = "resolved_at = NOW()"
	}
	args := []interface{}{shortID, status}
	collegeClause := ""
	if collegeID != "" {
		collegeClause = ` AND t.user_id IN (SELECT id FROM users WHERE college_id = $3::UUID)`
		args = append(args, collegeID)
	}
	tag, err := r.pool.Exec(ctx, fmt.Sprintf(`
		UPDATE support_tickets t SET status = $2::support_ticket_status, %s, updated_at = NOW()
		WHERE t.short_id = $1%s`, resolvedAtClause, collegeClause),
		args...,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

