-- Keep the local draft corpus distinguishable from reviewed legislation.
-- IF NOT EXISTS also converges local test databases that briefly received
-- these additions in the development copy of migration 000024.
ALTER TABLE legislation_versions
 ADD COLUMN IF NOT EXISTS test_only boolean NOT NULL DEFAULT false;

DO $$
BEGIN
 IF NOT EXISTS (
  SELECT 1 FROM pg_constraint
  WHERE conrelid='accounting_analysis_reviews'::regclass
    AND conname='accounting_analysis_reviews_analysis_id_key'
 ) THEN
  ALTER TABLE accounting_analysis_reviews
   ADD CONSTRAINT accounting_analysis_reviews_analysis_id_key UNIQUE(analysis_id);
 END IF;
END;
$$;
