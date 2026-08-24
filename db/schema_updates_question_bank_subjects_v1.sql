-- Question Bank subjects become DB-editable rows instead of a fixed list
-- baked into Go source (models/question_bank_taxonomy.go). Topics/subtopics
-- remain free-text on individual questions as before — only the top-level
-- subject tiles on the browse page move to this table, via an "Add Subject"
-- admin flow.
CREATE TABLE IF NOT EXISTS question_bank_subjects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_id TEXT UNIQUE NOT NULL,
    name TEXT UNIQUE NOT NULL,
    display_order INTEGER NOT NULL DEFAULT 0,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed the 5 subjects that already exist as the hardcoded taxonomy, so
-- nothing disappears from the browse page the moment this ships. Their
-- curated Topic/Subtopic trees stay in Go code (matched by name) — only
-- brand-new subjects created via the admin UI start with no preset topics.
INSERT INTO question_bank_subjects (short_id, name, display_order) VALUES
    ('7F3A9C1D', 'Python',     1),
    ('2B8E4F60', 'Java',       2),
    ('C15D7A22', 'C',          3),
    ('9E4B3D71', 'React',      4),
    ('F0A6C842', 'Full Stack', 5)
ON CONFLICT (name) DO NOTHING;
