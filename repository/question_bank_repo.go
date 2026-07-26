package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/util"
)

type QuestionBankRepository struct {
	pool *pgxpool.Pool
}

func NewQuestionBankRepository(pool *pgxpool.Pool) *QuestionBankRepository {
	return &QuestionBankRepository{pool: pool}
}

const questionBaseSelect = `
	SELECT q.id, q.short_id, q.question_type::TEXT, COALESCE(q.question_text, ''),
	       q.options, q.correct_option_ids, COALESCE(q.correct_text, ''), COALESCE(q.explanation, ''),
	       q.marks, q.negative_marks, COALESCE(q.subject, ''), COALESCE(q.topic, ''), COALESCE(q.subtopic, ''), COALESCE(q.difficulty, ''),
	       COALESCE(cq.short_id, ''), COALESCE(cq.title, ''),
	       q.visibility::TEXT, q.created_by, CONCAT(u.first_name, ' ', u.last_name),
	       q.created_at, q.updated_at
	FROM assessment_questions q
	JOIN users u ON q.created_by = u.id AND u.deleted_at IS NULL
	LEFT JOIN coding_questions cq ON q.coding_question_id = cq.id AND cq.deleted_at IS NULL`

func scanAssessmentQuestion(row pgx.Row) (models.Question, error) {
	var q models.Question
	var optionsRaw []byte
	err := row.Scan(
		&q.ID, &q.ShortID, &q.QuestionType, &q.QuestionText,
		&optionsRaw, &q.CorrectOptionIDs, &q.CorrectText, &q.Explanation,
		&q.Marks, &q.NegativeMarks, &q.Subject, &q.Topic, &q.Subtopic, &q.Difficulty,
		&q.CodingQuestionShortID, &q.CodingQuestionTitle,
		&q.Visibility, &q.CreatedBy, &q.CreatedByName,
		&q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		return q, err
	}
	if len(optionsRaw) > 0 {
		if err := json.Unmarshal(optionsRaw, &q.Options); err != nil {
			q.Options = []models.QuestionOption{}
		}
	}
	if q.Options == nil {
		q.Options = []models.QuestionOption{}
	}
	if q.CorrectOptionIDs == nil {
		q.CorrectOptionIDs = []string{}
	}
	return q, nil
}

func (r *QuestionBankRepository) Create(ctx context.Context, in models.CreateQuestionInput, createdBy string) (*models.Question, error) {
	options := in.Options
	if options == nil {
		options = []models.QuestionOption{}
	}
	optionsJSON, _ := json.Marshal(options)

	correctOptionIDs := in.CorrectOptionIDs
	if correctOptionIDs == nil {
		correctOptionIDs = []string{}
	}


	for attempt := 0; attempt < 3; attempt++ {
		shortID := util.GenerateShortID()

		// A data-modifying CTE and the main query share one snapshot, so a
		// plain re-scan of assessment_questions (e.g. "WHERE q.id = (SELECT
		// id FROM ins)") can never see the row ins just inserted — Postgres
		// only guarantees visibility through the CTE's own RETURNING
		// columns. So the outer SELECT reads FROM ins directly instead of
		// FROM assessment_questions q.
		q, err := scanAssessmentQuestion(r.pool.QueryRow(ctx, `
			WITH ins AS (
				INSERT INTO assessment_questions (
					short_id, question_type, question_text, options, correct_option_ids,
					correct_text, explanation, marks, negative_marks, subject, topic, subtopic, difficulty,
					coding_question_id, visibility, created_by
				) VALUES (
					$1, $2::assessment_question_type, NULLIF($3,''), $4, $5,
					NULLIF($6,''), NULLIF($7,''), $8, $9, NULLIF($10,''), NULLIF($11,''), NULLIF($12,''), NULLIF($13,''),
					(SELECT id FROM coding_questions WHERE short_id = NULLIF($14,'') AND deleted_at IS NULL),
					$15::question_visibility, $16
				)
				RETURNING *
			)
			SELECT ins.id, ins.short_id, ins.question_type::TEXT, COALESCE(ins.question_text, ''),
			       ins.options, ins.correct_option_ids, COALESCE(ins.correct_text, ''), COALESCE(ins.explanation, ''),
			       ins.marks, ins.negative_marks, COALESCE(ins.subject, ''), COALESCE(ins.topic, ''), COALESCE(ins.subtopic, ''), COALESCE(ins.difficulty, ''),
			       COALESCE(cq.short_id, ''), COALESCE(cq.title, ''),
			       ins.visibility::TEXT, ins.created_by, CONCAT(u.first_name, ' ', u.last_name),
			       ins.created_at, ins.updated_at
			FROM ins
			JOIN users u ON ins.created_by = u.id AND u.deleted_at IS NULL
			LEFT JOIN coding_questions cq ON ins.coding_question_id = cq.id AND cq.deleted_at IS NULL`,
			shortID, in.QuestionType, in.QuestionText, optionsJSON, correctOptionIDs,
			in.CorrectText, in.Explanation, in.Marks, in.NegativeMarks, in.Subject, in.Topic, in.Subtopic, in.Difficulty,
			in.CodingQuestionShortID, visibilityOrDefault(in.Visibility), createdBy,
		))
		if err == nil {
			return &q, nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" {
				continue
			}
			return nil, fmt.Errorf("insert question: %s (column=%s, detail=%s)", pgErr.Message, pgErr.ColumnName, pgErr.Detail)
		}
		return nil, fmt.Errorf("insert question: %w", err)
	}
	return nil, fmt.Errorf("could not generate a unique short ID after 3 attempts")
}

func visibilityOrDefault(v string) string {
	if v == "" {
		return "private"
	}
	return v
}

func (r *QuestionBankRepository) scanAll(ctx context.Context, q string, args ...interface{}) ([]models.Question, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Question
	for rows.Next() {
		item, err := scanAssessmentQuestion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// FindAll returns questions visible to the given creator: their own private
// questions, plus anything marked course/global visibility. Staff (super_admin
// / team_lead) can pass an empty creatorID to see everything.
func (r *QuestionBankRepository) FindAll(ctx context.Context, f models.QuestionFilter, creatorID string) ([]models.Question, error) {
	where := []string{"q.deleted_at IS NULL"}
	args := []interface{}{}
	i := 1

	if creatorID != "" {
		where = append(where, fmt.Sprintf("(q.visibility != 'private' OR q.created_by = $%d)", i))
		args = append(args, creatorID)
		i++
	}
	if f.QuestionType != "" {
		where = append(where, fmt.Sprintf("q.question_type = $%d::assessment_question_type", i))
		args = append(args, f.QuestionType)
		i++
	}
	if f.Subject != "" {
		where = append(where, fmt.Sprintf("q.subject = $%d", i))
		args = append(args, f.Subject)
		i++
	}
	if f.Topic != "" {
		where = append(where, fmt.Sprintf("q.topic ILIKE $%d", i))
		args = append(args, "%"+f.Topic+"%")
		i++
	}
	if f.Subtopic != "" {
		where = append(where, fmt.Sprintf("q.subtopic = $%d", i))
		args = append(args, f.Subtopic)
		i++
	}
	if f.Difficulty != "" {
		where = append(where, fmt.Sprintf("q.difficulty = $%d", i))
		args = append(args, f.Difficulty)
		i++
	}

	q := fmt.Sprintf("%s WHERE %s ORDER BY q.created_at DESC", questionBaseSelect, strings.Join(where, " AND "))
	return r.scanAll(ctx, q, args...)
}

func (r *QuestionBankRepository) FindByShortID(ctx context.Context, shortID string) (*models.Question, error) {
	q := fmt.Sprintf("%s WHERE q.short_id = $1 AND q.deleted_at IS NULL", questionBaseSelect)
	item, err := scanAssessmentQuestion(r.pool.QueryRow(ctx, q, shortID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &item, err
}

func (r *QuestionBankRepository) Update(ctx context.Context, shortID string, in models.UpdateQuestionInput) error {
	args := []interface{}{shortID}
	setClauses := []string{}
	i := 2

	add := func(clause string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf(clause, i))
		args = append(args, val)
		i++
	}

	if in.QuestionText != nil {
		add("question_text = NULLIF($%d,'')", *in.QuestionText)
	}
	if in.Options != nil {
		b, _ := json.Marshal(in.Options)
		add("options = $%d", b)
	}
	if in.CorrectOptionIDs != nil {
		add("correct_option_ids = $%d", in.CorrectOptionIDs)
	}
	if in.CorrectText != nil {
		add("correct_text = NULLIF($%d,'')", *in.CorrectText)
	}
	if in.Explanation != nil {
		add("explanation = NULLIF($%d,'')", *in.Explanation)
	}
	if in.Marks != nil {
		add("marks = $%d", *in.Marks)
	}
	if in.NegativeMarks != nil {
		add("negative_marks = $%d", *in.NegativeMarks)
	}
	if in.Subject != nil {
		add("subject = NULLIF($%d,'')", *in.Subject)
	}
	if in.Topic != nil {
		add("topic = NULLIF($%d,'')", *in.Topic)
	}
	if in.Subtopic != nil {
		add("subtopic = NULLIF($%d,'')", *in.Subtopic)
	}
	if in.Difficulty != nil {
		add("difficulty = NULLIF($%d,'')", *in.Difficulty)
	}
	if in.CodingQuestionShortID != nil {
		add("coding_question_id = (SELECT id FROM coding_questions WHERE short_id = NULLIF($%d,'') AND deleted_at IS NULL)", *in.CodingQuestionShortID)
	}
	if in.Visibility != nil {
		add("visibility = $%d::question_visibility", *in.Visibility)
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}
	setClauses = append(setClauses, "updated_at = NOW()")

	q := fmt.Sprintf("UPDATE assessment_questions SET %s WHERE short_id = $1 AND deleted_at IS NULL", strings.Join(setClauses, ", "))
	result, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *QuestionBankRepository) Delete(ctx context.Context, shortID string) error {
	result, err := r.pool.Exec(ctx, `UPDATE assessment_questions SET deleted_at = NOW() WHERE short_id = $1 AND deleted_at IS NULL`, shortID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ── Assessment ↔ Question linking ────────────────────────────────────────────

// AttachQuestion links an existing question-bank entry to an assessment.
func (r *QuestionBankRepository) AttachQuestion(ctx context.Context, assessmentShortID string, in models.AttachQuestionInput) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO assessment_question_links (assessment_id, question_id, order_index, marks_override)
		SELECT a.id, q.id, $3, $4
		FROM assessments a, assessment_questions q
		WHERE a.short_id = $1 AND a.deleted_at IS NULL
		  AND q.short_id = $2 AND q.deleted_at IS NULL
		ON CONFLICT (assessment_id, question_id) DO UPDATE SET
			order_index = EXCLUDED.order_index,
			marks_override = EXCLUDED.marks_override`,
		assessmentShortID, in.QuestionShortID, in.OrderIndex, in.MarksOverride,
	)
	return err
}

// GetQuestions returns every question attached to an assessment, in display order.
func (r *QuestionBankRepository) GetQuestions(ctx context.Context, assessmentShortID string) ([]models.AssessmentQuestion, error) {
	q := `
		SELECT aql.order_index, aql.marks_override,
		       q.id, q.short_id, q.question_type::TEXT, COALESCE(q.question_text, ''),
		       q.options, q.correct_option_ids, COALESCE(q.correct_text, ''), COALESCE(q.explanation, ''),
		       q.marks, q.negative_marks, COALESCE(q.subject, ''), COALESCE(q.topic, ''), COALESCE(q.subtopic, ''), COALESCE(q.difficulty, ''),
		       COALESCE(cq.short_id, ''), COALESCE(cq.title, ''),
		       q.visibility::TEXT, q.created_by, CONCAT(u.first_name, ' ', u.last_name),
		       q.created_at, q.updated_at
		FROM assessment_question_links aql
		JOIN assessments a ON aql.assessment_id = a.id
		JOIN assessment_questions q ON aql.question_id = q.id AND q.deleted_at IS NULL
		JOIN users u ON q.created_by = u.id AND u.deleted_at IS NULL
		LEFT JOIN coding_questions cq ON q.coding_question_id = cq.id AND cq.deleted_at IS NULL
		WHERE a.short_id = $1
		ORDER BY aql.order_index ASC`

	rows, err := r.pool.Query(ctx, q, assessmentShortID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.AssessmentQuestion
	for rows.Next() {
		var aq models.AssessmentQuestion
		var optionsRaw []byte
		if err := rows.Scan(
			&aq.OrderIndex, &aq.MarksOverride,
			&aq.ID, &aq.ShortID, &aq.QuestionType, &aq.QuestionText,
			&optionsRaw, &aq.CorrectOptionIDs, &aq.CorrectText, &aq.Explanation,
			&aq.Marks, &aq.NegativeMarks, &aq.Subject, &aq.Topic, &aq.Subtopic, &aq.Difficulty,
			&aq.CodingQuestionShortID, &aq.CodingQuestionTitle,
			&aq.Visibility, &aq.CreatedBy, &aq.CreatedByName,
			&aq.CreatedAt, &aq.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if len(optionsRaw) > 0 {
			if err := json.Unmarshal(optionsRaw, &aq.Options); err != nil {
				aq.Options = []models.QuestionOption{}
			}
		}
		if aq.Options == nil {
			aq.Options = []models.QuestionOption{}
		}
		if aq.CorrectOptionIDs == nil {
			aq.CorrectOptionIDs = []string{}
		}
		out = append(out, aq)
	}
	return out, rows.Err()
}

func (r *QuestionBankRepository) UpdateAttachedQuestion(ctx context.Context, assessmentShortID, questionShortID string, in models.UpdateAttachedQuestionInput) error {
	args := []interface{}{assessmentShortID, questionShortID}
	setClauses := []string{}
	i := 3
	add := func(clause string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf(clause, i))
		args = append(args, val)
		i++
	}
	if in.OrderIndex != nil {
		add("order_index = $%d", *in.OrderIndex)
	}
	if in.MarksOverride != nil {
		add("marks_override = $%d", *in.MarksOverride)
	}
	if len(setClauses) == 0 {
		return fmt.Errorf("no fields to update")
	}

	q := fmt.Sprintf(`
		UPDATE assessment_question_links SET %s
		WHERE assessment_id = (SELECT id FROM assessments WHERE short_id = $1 AND deleted_at IS NULL)
		  AND question_id   = (SELECT id FROM assessment_questions WHERE short_id = $2 AND deleted_at IS NULL)`,
		strings.Join(setClauses, ", "))
	result, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *QuestionBankRepository) DetachQuestion(ctx context.Context, assessmentShortID, questionShortID string) error {
	result, err := r.pool.Exec(ctx, `
		DELETE FROM assessment_question_links
		WHERE assessment_id = (SELECT id FROM assessments WHERE short_id = $1 AND deleted_at IS NULL)
		  AND question_id   = (SELECT id FROM assessment_questions WHERE short_id = $2 AND deleted_at IS NULL)`,
		assessmentShortID, questionShortID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// GetStats aggregates counts for one subject — total, by question_type, by
// difficulty, and by topic — powering the subject browse cards.
func (r *QuestionBankRepository) GetStats(ctx context.Context, subject string) (*models.QuestionBankStats, error) {
	stats := &models.QuestionBankStats{
		Subject:        subject,
		ByQuestionType: map[string]int{},
		ByDifficulty:   map[string]int{},
	}

	rows, err := r.pool.Query(ctx, `
		SELECT question_type::TEXT, COALESCE(difficulty, ''), COALESCE(topic, ''), count(*)
		FROM assessment_questions
		WHERE subject = $1 AND deleted_at IS NULL
		GROUP BY question_type, difficulty, topic`, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	topicCounts := map[string]int{}
	for rows.Next() {
		var qType, difficulty, topic string
		var count int
		if err := rows.Scan(&qType, &difficulty, &topic, &count); err != nil {
			return nil, err
		}
		stats.Total += count
		stats.ByQuestionType[qType] += count
		if difficulty != "" {
			stats.ByDifficulty[difficulty] += count
		}
		if topic != "" {
			topicCounts[topic] += count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for topic, count := range topicCounts {
		stats.Topics = append(stats.Topics, models.TopicCount{Topic: topic, Count: count})
	}
	return stats, nil
}
