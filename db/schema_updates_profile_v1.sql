-- Personal profile-details extension (bio, location, education, skills, socials)
-- that the student app's Profile page needs but which doesn't belong on the
-- core `users` row. One row per user, created on first save.
CREATE TABLE IF NOT EXISTS profile_details (
    user_id      UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    bio          TEXT NOT NULL DEFAULT '',
    location     VARCHAR(255) NOT NULL DEFAULT '',
    education    VARCHAR(255) NOT NULL DEFAULT '',
    skills       TEXT[] NOT NULL DEFAULT '{}',
    linkedin_url TEXT NOT NULL DEFAULT '',
    github_url   TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
