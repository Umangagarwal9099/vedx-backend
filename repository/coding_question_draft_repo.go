package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type CodingQuestionDraftRepository struct {
	pool *pgxpool.Pool
}

func NewCodingQuestionDraftRepository(pool *pgxpool.Pool) *CodingQuestionDraftRepository {
	return &CodingQuestionDraftRepository{pool: pool}
}

// Upsert saves (or overwrites) the caller's latest draft for a question+
// language — called on every autosave tick, cheap to call repeatedly.
func (r *CodingQuestionDraftRepository) Upsert(ctx context.Context, userID, questionID, language, code string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO coding_question_drafts (user_id, question_id, language, code, updated_at)
		VALUES ($1::UUID, $2::UUID, $3, $4, NOW())
		ON CONFLICT (user_id, question_id, language) DO UPDATE SET
			code = EXCLUDED.code,
			updated_at = NOW()`,
		userID, questionID, language, code,
	)
	return err
}

// GetAllForQuestion returns every saved draft (one per language) the caller
// has for a question, keyed by language — fetched once per question visit
// so switching the language toggle client-side never needs another request.
func (r *CodingQuestionDraftRepository) GetAllForQuestion(ctx context.Context, userID, questionID string) (map[string]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT language, code FROM coding_question_drafts
		WHERE user_id = $1::UUID AND question_id = $2::UUID`,
		userID, questionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var lang, code string
		if err := rows.Scan(&lang, &code); err != nil {
			return nil, err
		}
		out[lang] = code
	}
	return out, rows.Err()
}
