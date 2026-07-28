package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
)

type SubmissionRepository struct {
	pool *pgxpool.Pool
}

func NewSubmissionRepository(pool *pgxpool.Pool) *SubmissionRepository {
	return &SubmissionRepository{pool: pool}
}

func (r *SubmissionRepository) Create(ctx context.Context, userID string, in models.CreateSubmissionInput) (*models.Submission, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO submissions (user_id, question_short_id, language, code, status, passed_tests, total_tests)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, question_short_id, language, code, status, passed_tests, total_tests, created_at`,
		userID, in.QuestionShortID, in.Language, in.Code, in.Status, in.PassedTests, in.TotalTests,
	)
	var s models.Submission
	if err := row.Scan(&s.ID, &s.UserID, &s.QuestionShortID, &s.Language, &s.Code, &s.Status, &s.PassedTests, &s.TotalTests, &s.CreatedAt); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SubmissionRepository) FindByUser(ctx context.Context, userID string) ([]models.Submission, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, question_short_id, language, code, status, passed_tests, total_tests, created_at
		FROM submissions
		WHERE user_id = $1
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Submission
	for rows.Next() {
		var s models.Submission
		if err := rows.Scan(&s.ID, &s.UserID, &s.QuestionShortID, &s.Language, &s.Code, &s.Status, &s.PassedTests, &s.TotalTests, &s.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, nil
}

// FindAll returns all submissions joined with student and question info — for admin/mentor views.
func (r *SubmissionRepository) FindAll(ctx context.Context) ([]models.SubmissionView, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, s.user_id,
		       u.first_name || ' ' || u.last_name AS student_name, u.email AS student_email,
		       s.question_short_id, cq.title AS question_title,
		       s.language, s.code, s.status, s.passed_tests, s.total_tests, s.created_at
		FROM submissions s
		JOIN users u              ON u.id       = s.user_id
		JOIN coding_questions cq  ON cq.short_id = s.question_short_id
		ORDER BY s.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.SubmissionView
	for rows.Next() {
		var v models.SubmissionView
		if err := rows.Scan(&v.ID, &v.UserID, &v.StudentName, &v.StudentEmail,
			&v.QuestionShortID, &v.QuestionTitle, &v.Language, &v.Code,
			&v.Status, &v.PassedTests, &v.TotalTests, &v.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, nil
}

// FindByUserID returns all submissions for one user joined with question info — for admin/mentor detail view.
func (r *SubmissionRepository) FindByUserID(ctx context.Context, userID string) ([]models.SubmissionView, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, s.user_id,
		       u.first_name || ' ' || u.last_name AS student_name, u.email AS student_email,
		       s.question_short_id, cq.title AS question_title,
		       s.language, s.code, s.status, s.passed_tests, s.total_tests, s.created_at
		FROM submissions s
		JOIN users u              ON u.id       = s.user_id
		JOIN coding_questions cq  ON cq.short_id = s.question_short_id
		WHERE s.user_id = $1
		ORDER BY s.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.SubmissionView
	for rows.Next() {
		var v models.SubmissionView
		if err := rows.Scan(&v.ID, &v.UserID, &v.StudentName, &v.StudentEmail,
			&v.QuestionShortID, &v.QuestionTitle, &v.Language, &v.Code,
			&v.Status, &v.PassedTests, &v.TotalTests, &v.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, nil
}

func (r *SubmissionRepository) FindByUserAndQuestion(ctx context.Context, userID, questionShortID string) ([]models.Submission, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, question_short_id, language, code, status, passed_tests, total_tests, created_at
		FROM submissions
		WHERE user_id = $1 AND question_short_id = $2
		ORDER BY created_at DESC`, userID, questionShortID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Submission
	for rows.Next() {
		var s models.Submission
		if err := rows.Scan(&s.ID, &s.UserID, &s.QuestionShortID, &s.Language, &s.Code, &s.Status, &s.PassedTests, &s.TotalTests, &s.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, nil
}

// GetLeaderboard ranks students by distinct problems solved (accepted
// submissions), most-solved first. collegeID scopes the ranking to peers
// within that college — "peer ranking" only means something among students
// who actually share a college; empty collegeID (super_admin/unscoped)
// ranks across every student on the platform. Only students who have solved
// at least one problem show up — no zero-solved noise at the bottom.
func (r *SubmissionRepository) GetLeaderboard(ctx context.Context, collegeID string) ([]models.LeaderboardEntry, error) {
	args := []interface{}{}
	collegeClause := ""
	if collegeID != "" {
		collegeClause = " AND u.college_id = $1::UUID"
		args = append(args, collegeID)
	}

	q := fmt.Sprintf(`
		SELECT u.id, u.first_name || ' ' || u.last_name AS student_name, COALESCE(u.college_id::TEXT, ''),
		       COUNT(DISTINCT CASE WHEN s.status = 'accepted' THEN s.question_short_id END) AS solved_count,
		       COUNT(CASE WHEN s.status = 'accepted' THEN 1 END) AS accepted_submissions,
		       COUNT(*) AS total_submissions,
		       MAX(CASE WHEN s.status = 'accepted' THEN s.created_at END) AS last_solved_at
		FROM submissions s
		JOIN users u ON u.id = s.user_id AND u.deleted_at IS NULL AND u.role = 'student'
		WHERE TRUE%s
		GROUP BY u.id, u.first_name, u.last_name
		HAVING COUNT(DISTINCT CASE WHEN s.status = 'accepted' THEN s.question_short_id END) > 0
		ORDER BY solved_count DESC, accepted_submissions ASC, student_name ASC
		LIMIT 100`, collegeClause)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.LeaderboardEntry
	for rows.Next() {
		var e models.LeaderboardEntry
		if err := rows.Scan(&e.StudentID, &e.StudentName, &e.CollegeID, &e.SolvedCount, &e.AcceptedSubmissions, &e.TotalSubmissions, &e.LastSolvedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Standard competition ranking: ties (same solved_count + accepted_submissions)
	// share a rank, and the next distinct rank skips ahead accordingly (1, 2, 2, 4).
	rank := 0
	for i := range entries {
		if i == 0 || entries[i].SolvedCount != entries[i-1].SolvedCount || entries[i].AcceptedSubmissions != entries[i-1].AcceptedSubmissions {
			rank = i + 1
		}
		entries[i].Rank = rank
	}
	return entries, nil
}
