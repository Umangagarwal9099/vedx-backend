package repository

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ActivityRepository struct {
	pool *pgxpool.Pool
}

func NewActivityRepository(pool *pgxpool.Pool) *ActivityRepository {
	return &ActivityRepository{pool: pool}
}

const activityDateLayout = "2006-01-02"

// getActiveDates returns every distinct calendar date on which the student
// did something countable: submitted an assignment/project (individually or
// via a team), attempted an exam, or was marked present/late at a session.
// Dates come back newest first.
func (r *ActivityRepository) getActiveDates(ctx context.Context, studentID string) ([]time.Time, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT d FROM (
			SELECT submitted_at::DATE AS d FROM assignment_submissions WHERE student_id = $1::UUID
			UNION
			SELECT ps.submitted_at::DATE
			FROM project_submissions ps
			LEFT JOIN project_team_members ptm ON ps.team_id = ptm.team_id
			WHERE ps.student_id = $1::UUID OR ptm.user_id = $1::UUID
			UNION
			SELECT started_at::DATE FROM exam_attempts WHERE student_id = $1::UUID
			UNION
			SELECT s.session_date
			FROM session_attendance sa
			JOIN sessions s ON sa.session_id = s.id
			WHERE sa.student_id = $1::UUID AND sa.status IN ('present', 'late')
		) all_dates
		ORDER BY d DESC`,
		studentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []time.Time
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// computeStreak derives current/longest streak from a newest-first list of
// distinct active dates. "Current" tolerates today not yet having activity —
// it only breaks once the most recent active day is more than 1 day ago.
func computeStreak(datesDesc []time.Time) (current, longest int, lastActive *time.Time) {
	if len(datesDesc) == 0 {
		return 0, 0, nil
	}
	lastActive = &datesDesc[0]

	today := time.Now().Truncate(24 * time.Hour)
	gapToToday := int(today.Sub(datesDesc[0].Truncate(24*time.Hour)).Hours() / 24)
	if gapToToday <= 1 {
		current = 1
		for i := 1; i < len(datesDesc); i++ {
			gap := int(datesDesc[i-1].Truncate(24*time.Hour).Sub(datesDesc[i].Truncate(24*time.Hour)).Hours() / 24)
			if gap == 1 {
				current++
			} else {
				break
			}
		}
	}

	longest = 1
	run := 1
	for i := 1; i < len(datesDesc); i++ {
		gap := int(datesDesc[i-1].Truncate(24*time.Hour).Sub(datesDesc[i].Truncate(24*time.Hour)).Hours() / 24)
		if gap == 1 {
			run++
		} else {
			run = 1
		}
		if run > longest {
			longest = run
		}
	}

	return current, longest, lastActive
}

// GetStreak computes a student's current streak, longest streak, and total
// active days from their submission/attempt/attendance history.
func (r *ActivityRepository) GetStreak(ctx context.Context, studentID string) (current, longest, total int, lastActiveDate *string, err error) {
	dates, err := r.getActiveDates(ctx, studentID)
	if err != nil {
		return 0, 0, 0, nil, err
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].After(dates[j]) })

	current, longest, lastActive := computeStreak(dates)
	if lastActive != nil {
		s := lastActive.Format(activityDateLayout)
		lastActiveDate = &s
	}
	return current, longest, len(dates), lastActiveDate, nil
}
