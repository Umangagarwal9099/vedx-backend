-- ============================================================================
-- assessment_question_links has two FKs, both created with Postgres's default
-- ON DELETE NO ACTION — meaning a hard DELETE on `assessments` or
-- `assessment_questions` is blocked while any link row still references it.
-- The app itself never hits this (both Delete endpoints soft-delete via
-- deleted_at), but a direct DB delete does.
--
-- Fix: cascade only the link rows, never the underlying question-bank rows.
-- Deleting an assessment removes its "which questions are attached" wiring;
-- the actual reusable questions in assessment_questions are untouched either
-- way, since nothing here ever cascades INTO that table.
-- ============================================================================

ALTER TABLE assessment_question_links
    DROP CONSTRAINT assessment_question_links_assessment_id_fkey,
    ADD CONSTRAINT assessment_question_links_assessment_id_fkey
        FOREIGN KEY (assessment_id) REFERENCES assessments(id) ON DELETE CASCADE;

ALTER TABLE assessment_question_links
    DROP CONSTRAINT assessment_question_links_question_id_fkey,
    ADD CONSTRAINT assessment_question_links_question_id_fkey
        FOREIGN KEY (question_id) REFERENCES assessment_questions(id) ON DELETE CASCADE;

-- ============================================================================
-- End of file
-- ============================================================================
