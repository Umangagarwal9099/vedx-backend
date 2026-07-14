-- ============================================================================
-- Phase 3 of the LMS-flow audit: score weighting, ranking, and leaderboard.
-- No new tables — final scores are computed on demand from the existing
-- assignment_submissions / exam_attempts / project_submissions tables and
-- persisted into the final_score / final_rank columns student_enrollments
-- already has (added, unused, back in Phase 1). Fully additive.
-- ============================================================================

ALTER TABLE batches ADD COLUMN IF NOT EXISTS score_weight_assignments INT NOT NULL DEFAULT 40;
ALTER TABLE batches ADD COLUMN IF NOT EXISTS score_weight_exams       INT NOT NULL DEFAULT 40;
ALTER TABLE batches ADD COLUMN IF NOT EXISTS score_weight_projects    INT NOT NULL DEFAULT 20;

-- ============================================================================
-- End of file
-- ============================================================================
