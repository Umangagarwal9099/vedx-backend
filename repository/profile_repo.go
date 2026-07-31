package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/models"
)

type ProfileRepository struct {
	pool *pgxpool.Pool
}

func NewProfileRepository(pool *pgxpool.Pool) *ProfileRepository {
	return &ProfileRepository{pool: pool}
}

// GetByUserID returns the user's profile details, joined with their live
// phone number from `users`. Returns a zero-value (empty) ProfileDetails —
// never nil, never an error — if the user hasn't saved a profile yet, since
// "no profile row yet" isn't an error condition.
func (r *ProfileRepository) GetByUserID(ctx context.Context, userID string) (*models.ProfileDetails, error) {
	var p models.ProfileDetails
	err := r.pool.QueryRow(ctx, `
		SELECT u.id, u.phone,
		       COALESCE(pd.bio, ''), COALESCE(pd.location, ''), COALESCE(pd.education, ''),
		       COALESCE(pd.skills, '{}'), COALESCE(pd.linkedin_url, ''), COALESCE(pd.github_url, ''),
		       COALESCE(pd.resume_url, ''),
		       COALESCE(pd.updated_at, u.updated_at)
		FROM users u
		LEFT JOIN profile_details pd ON pd.user_id = u.id
		WHERE u.id = $1::UUID`,
		userID,
	).Scan(&p.UserID, &p.Phone, &p.Bio, &p.Location, &p.Education, &p.Skills, &p.LinkedInURL, &p.GithubURL, &p.ResumeURL, &p.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// Upsert creates or updates the profile_details row for userID with whatever
// fields are non-nil in the input. Phone is handled separately by the caller
// via UserRepository.UpdateUser, since it lives on `users`, not this table.
func (r *ProfileRepository) Upsert(ctx context.Context, userID string, in models.UpdateProfileDetailsInput) error {
	current, err := r.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}
	bio, location, education, linkedIn, github, resume := "", "", "", "", "", ""
	var skills []string
	if current != nil {
		bio, location, education, linkedIn, github, resume = current.Bio, current.Location, current.Education, current.LinkedInURL, current.GithubURL, current.ResumeURL
		skills = current.Skills
	}
	if in.Bio != nil {
		bio = *in.Bio
	}
	if in.Location != nil {
		location = *in.Location
	}
	if in.Education != nil {
		education = *in.Education
	}
	if in.Skills != nil {
		skills = *in.Skills
	}
	if in.LinkedInURL != nil {
		linkedIn = *in.LinkedInURL
	}
	if in.GithubURL != nil {
		github = *in.GithubURL
	}
	if in.ResumeURL != nil {
		resume = *in.ResumeURL
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO profile_details (user_id, bio, location, education, skills, linkedin_url, github_url, resume_url, updated_at)
		VALUES ($1::UUID, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			bio = EXCLUDED.bio, location = EXCLUDED.location, education = EXCLUDED.education,
			skills = EXCLUDED.skills, linkedin_url = EXCLUDED.linkedin_url, github_url = EXCLUDED.github_url,
			resume_url = EXCLUDED.resume_url,
			updated_at = NOW()`,
		userID, bio, location, education, skills, linkedIn, github, resume,
	)
	return err
}
