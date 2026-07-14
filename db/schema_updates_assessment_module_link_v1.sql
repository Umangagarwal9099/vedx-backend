-- Lets an assessment be tied to "the module I taught today", the same way
-- assignments already link to a module.
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS module_id UUID REFERENCES modules(id);
CREATE INDEX IF NOT EXISTS idx_assessments_module_id ON assessments(module_id);
