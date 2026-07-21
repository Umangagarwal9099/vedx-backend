-- ============================================================================
-- Community feed: posts, comments, and likes scoped to an existing
-- `communities` row (itself scoped to a batch). Fully additive — no existing
-- table is touched. Deliberately minimal for this pass: no attachments,
-- mentions, reports, or direct messages — those are a separate, larger
-- initiative (moderation queue + DM subsystem) flagged for later.
-- ============================================================================

CREATE TABLE IF NOT EXISTS community_posts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id      VARCHAR(16) NOT NULL UNIQUE,
    community_id  UUID NOT NULL REFERENCES communities(id),
    author_id     UUID NOT NULL REFERENCES users(id),
    content       TEXT NOT NULL,
    is_pinned     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_community_posts_community ON community_posts(community_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS community_comments (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id      VARCHAR(16) NOT NULL UNIQUE,
    post_id       UUID NOT NULL REFERENCES community_posts(id),
    author_id     UUID NOT NULL REFERENCES users(id),
    content       TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_community_comments_post ON community_comments(post_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS community_post_likes (
    post_id     UUID NOT NULL REFERENCES community_posts(id),
    user_id     UUID NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (post_id, user_id)
);
