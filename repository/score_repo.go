package repository

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
)

type ScoreRepository struct {
	pool *pgxpool.Pool
}

func NewScoreRepository(pool *pgxpool.Pool) *ScoreRepository {
	return &ScoreRepository{pool: pool}
}

func toCategoryScore(earned, max float64) models.CategoryScore {
	cs := models.CategoryScore{Earned: earned, Max: max}
	if max > 0 {
		cs.Percentage = earned / max * 100
	}
	return cs
}

// batchAssignmentTotals sums evaluated assignment marks (and their assignment's
// max_marks) per student, across every assignment in the batch.
func (r *ScoreRepository) batchAssignmentTotals(ctx context.Context, batchID string) (map[string]models.CategoryScore, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT asub.student_id, COALESCE(SUM(asub.marks), 0), COALESCE(SUM(a.max_marks), 0)
		FROM assignment_submissions asub
		JOIN assignments a ON asub.assignment_id = a.id
		WHERE a.batch_id = $1::UUID AND asub.status = 'evaluated'
		GROUP BY asub.student_id`,
		batchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]models.CategoryScore{}
	for rows.Next() {
		var studentID string
		var earned, max float64
		if err := rows.Scan(&studentID, &earned, &max); err != nil {
			return nil, err
		}
		out[studentID] = toCategoryScore(earned, max)
	}
	return out, rows.Err()
}

// batchExamTotals sums each student's BEST evaluated attempt per assessment
// (not every attempt — a reattempt shouldn't double-count), across every
// assessment scoped to the batch.
func (r *ScoreRepository) batchExamTotals(ctx context.Context, batchID string) (map[string]models.CategoryScore, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT best.student_id, COALESCE(SUM(best.total_score), 0), COALESCE(SUM(best.max_score), 0)
		FROM (
			SELECT DISTINCT ON (ea.assessment_id, ea.student_id)
			       ea.student_id, ea.assessment_id, ea.total_score, ea.max_score
			FROM exam_attempts ea
			JOIN assessments a ON ea.assessment_id = a.id
			WHERE a.batch_id = $1::UUID AND ea.status = 'evaluated'
			ORDER BY ea.assessment_id, ea.student_id, ea.total_score DESC
		) best
		GROUP BY best.student_id`,
		batchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]models.CategoryScore{}
	for rows.Next() {
		var studentID string
		var earned, max float64
		if err := rows.Scan(&studentID, &earned, &max); err != nil {
			return nil, err
		}
		out[studentID] = toCategoryScore(earned, max)
	}
	return out, rows.Err()
}

// batchProjectTotals sums evaluated project-milestone marks per student
// (crediting every member when the submission was made by a team), and the
// max_marks of every distinct project they have at least one evaluated
// submission in, across every project in the batch.
func (r *ScoreRepository) batchProjectTotals(ctx context.Context, batchID string) (map[string]models.CategoryScore, error) {
	rows, err := r.pool.Query(ctx, `
		WITH sp AS (
			SELECT COALESCE(ps.student_id, ptm.user_id) AS student_id,
			       p.id AS project_id, p.max_marks AS project_max_marks, ps.marks
			FROM project_submissions ps
			JOIN project_milestones m ON ps.milestone_id = m.id
			JOIN projects p ON m.project_id = p.id
			LEFT JOIN project_team_members ptm ON ps.team_id = ptm.team_id
			WHERE p.batch_id = $1::UUID AND ps.status = 'evaluated'
			  AND (ps.student_id IS NOT NULL OR ptm.user_id IS NOT NULL)
		),
		per_student_project AS (
			SELECT student_id, project_id, SUM(marks) AS earned, MAX(project_max_marks) AS max_marks
			FROM sp
			GROUP BY student_id, project_id
		)
		SELECT student_id, COALESCE(SUM(earned), 0), COALESCE(SUM(max_marks), 0)
		FROM per_student_project
		GROUP BY student_id`,
		batchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]models.CategoryScore{}
	for rows.Next() {
		var studentID string
		var earned, max float64
		if err := rows.Scan(&studentID, &earned, &max); err != nil {
			return nil, err
		}
		out[studentID] = toCategoryScore(earned, max)
	}
	return out, rows.Err()
}

// studentAssignmentTotals is the single-student version of batchAssignmentTotals.
func (r *ScoreRepository) studentAssignmentTotals(ctx context.Context, studentID, batchID string) (models.CategoryScore, error) {
	var earned, max float64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(asub.marks), 0), COALESCE(SUM(a.max_marks), 0)
		FROM assignment_submissions asub
		JOIN assignments a ON asub.assignment_id = a.id
		WHERE a.batch_id = $1::UUID AND asub.student_id = $2::UUID AND asub.status = 'evaluated'`,
		batchID, studentID,
	).Scan(&earned, &max)
	if err != nil {
		return models.CategoryScore{}, err
	}
	return toCategoryScore(earned, max), nil
}

// studentExamTotals is the single-student version of batchExamTotals.
func (r *ScoreRepository) studentExamTotals(ctx context.Context, studentID, batchID string) (models.CategoryScore, error) {
	var earned, max float64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(best.total_score), 0), COALESCE(SUM(best.max_score), 0)
		FROM (
			SELECT DISTINCT ON (ea.assessment_id)
			       ea.total_score, ea.max_score
			FROM exam_attempts ea
			JOIN assessments a ON ea.assessment_id = a.id
			WHERE a.batch_id = $1::UUID AND ea.student_id = $2::UUID AND ea.status = 'evaluated'
			ORDER BY ea.assessment_id, ea.total_score DESC
		) best`,
		batchID, studentID,
	).Scan(&earned, &max)
	if err != nil {
		return models.CategoryScore{}, err
	}
	return toCategoryScore(earned, max), nil
}

// studentProjectTotals is the single-student version of batchProjectTotals.
func (r *ScoreRepository) studentProjectTotals(ctx context.Context, studentID, batchID string) (models.CategoryScore, error) {
	var earned, max float64
	err := r.pool.QueryRow(ctx, `
		WITH sp AS (
			SELECT p.id AS project_id, p.max_marks AS project_max_marks, ps.marks
			FROM project_submissions ps
			JOIN project_milestones m ON ps.milestone_id = m.id
			JOIN projects p ON m.project_id = p.id
			LEFT JOIN project_team_members ptm ON ps.team_id = ptm.team_id
			WHERE p.batch_id = $1::UUID AND ps.status = 'evaluated'
			  AND (ps.student_id = $2::UUID OR ptm.user_id = $2::UUID)
		),
		per_project AS (
			SELECT project_id, SUM(marks) AS earned, MAX(project_max_marks) AS max_marks
			FROM sp
			GROUP BY project_id
		)
		SELECT COALESCE(SUM(earned), 0), COALESCE(SUM(max_marks), 0) FROM per_project`,
		batchID, studentID,
	).Scan(&earned, &max)
	if err != nil {
		return models.CategoryScore{}, err
	}
	return toCategoryScore(earned, max), nil
}

func weightedFinalScore(assignments, exams, projects models.CategoryScore, aw, ew, pw int) float64 {
	totalWeight := aw + ew + pw
	if totalWeight == 0 {
		return 0
	}
	weighted := assignments.Percentage*float64(aw) + exams.Percentage*float64(ew) + projects.Percentage*float64(pw)
	return weighted / float64(totalWeight)
}

// assignRanks sorts breakdowns by FinalScore descending and assigns
// competition ranking (ties share a rank; the next rank skips accordingly).
func assignRanks(breakdowns []models.StudentScoreBreakdown) {
	sort.Slice(breakdowns, func(i, j int) bool { return breakdowns[i].FinalScore > breakdowns[j].FinalScore })
	for i := range breakdowns {
		if i > 0 && breakdowns[i].FinalScore == breakdowns[i-1].FinalScore {
			breakdowns[i].Rank = breakdowns[i-1].Rank
		} else {
			breakdowns[i].Rank = i + 1
		}
	}
}

type rosterEntry struct {
	studentID   string
	studentName string
}

func (r *ScoreRepository) getRoster(ctx context.Context, batchID string) ([]rosterEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT bs.user_id, CONCAT(u.first_name, ' ', u.last_name)
		FROM batch_students bs
		JOIN users u ON bs.user_id = u.id
		WHERE bs.batch_id = $1::UUID`,
		batchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []rosterEntry
	for rows.Next() {
		var e rosterEntry
		if err := rows.Scan(&e.studentID, &e.studentName); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetBatchLeaderboard computes every enrolled student's weighted final score
// and rank for a batch, persists the result into student_enrollments
// (final_score / final_rank), and returns the sorted leaderboard.
func (r *ScoreRepository) GetBatchLeaderboard(ctx context.Context, batchID, batchShortID, batchNumber string, aw, ew, pw int) ([]models.StudentScoreBreakdown, error) {
	roster, err := r.getRoster(ctx, batchID)
	if err != nil {
		return nil, err
	}

	assignments, err := r.batchAssignmentTotals(ctx, batchID)
	if err != nil {
		return nil, err
	}
	exams, err := r.batchExamTotals(ctx, batchID)
	if err != nil {
		return nil, err
	}
	projects, err := r.batchProjectTotals(ctx, batchID)
	if err != nil {
		return nil, err
	}

	breakdowns := make([]models.StudentScoreBreakdown, 0, len(roster))
	for _, entry := range roster {
		b := models.StudentScoreBreakdown{
			StudentID:    entry.studentID,
			StudentName:  entry.studentName,
			BatchID:      batchID,
			BatchShortID: batchShortID,
			BatchNumber:  batchNumber,
			Assignments:  assignments[entry.studentID],
			Exams:        exams[entry.studentID],
			Projects:     projects[entry.studentID],
		}
		b.FinalScore = weightedFinalScore(b.Assignments, b.Exams, b.Projects, aw, ew, pw)
		breakdowns = append(breakdowns, b)
	}
	assignRanks(breakdowns)

	if err := r.persistRanks(ctx, batchID, breakdowns); err != nil {
		return nil, err
	}

	return breakdowns, nil
}

// persistRanks writes each student's computed final_score/final_rank back
// into their student_enrollments row for this batch, so other features that
// read the enrollment table see an up-to-date rank without recomputing it.
func (r *ScoreRepository) persistRanks(ctx context.Context, batchID string, breakdowns []models.StudentScoreBreakdown) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, b := range breakdowns {
		if _, err := tx.Exec(ctx, `
			UPDATE student_enrollments SET final_score = $3, final_rank = $4, updated_at = NOW()
			WHERE student_id = $1::UUID AND batch_id = $2::UUID`,
			b.StudentID, batchID, b.FinalScore, b.Rank,
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// GetStudentBreakdown computes one student's score breakdown for a single
// batch, without touching the leaderboard-wide rank (rank is only meaningful
// batch-wide — see GetBatchLeaderboard).
func (r *ScoreRepository) GetStudentBreakdown(ctx context.Context, studentID, batchID, batchShortID, batchNumber string, aw, ew, pw int) (models.StudentScoreBreakdown, error) {
	assignments, err := r.studentAssignmentTotals(ctx, studentID, batchID)
	if err != nil {
		return models.StudentScoreBreakdown{}, err
	}
	exams, err := r.studentExamTotals(ctx, studentID, batchID)
	if err != nil {
		return models.StudentScoreBreakdown{}, err
	}
	projects, err := r.studentProjectTotals(ctx, studentID, batchID)
	if err != nil {
		return models.StudentScoreBreakdown{}, err
	}

	b := models.StudentScoreBreakdown{
		StudentID:    studentID,
		BatchID:      batchID,
		BatchShortID: batchShortID,
		BatchNumber:  batchNumber,
		Assignments:  assignments,
		Exams:        exams,
		Projects:     projects,
	}
	b.FinalScore = weightedFinalScore(assignments, exams, projects, aw, ew, pw)
	return b, nil
}
