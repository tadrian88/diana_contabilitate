ALTER TABLE validation_tasks
 ADD COLUMN classification_run_id text REFERENCES classification_runs(id);

UPDATE validation_tasks t
SET classification_run_id=i.current_classification_run_id
FROM invoices i
WHERE t.invoice_id=i.id
  AND t.task_type='CLASSIFICATION'
  AND t.status<>'RESOLVED'
  AND t.classification_run_id IS NULL;

CREATE UNIQUE INDEX validation_tasks_classification_run_active
 ON validation_tasks(classification_run_id)
 WHERE classification_run_id IS NOT NULL AND task_type='CLASSIFICATION';

ALTER TABLE accounting_analysis_runs DROP CONSTRAINT accounting_analysis_runs_status_check;
ALTER TABLE accounting_analysis_runs ADD CONSTRAINT accounting_analysis_runs_status_check
 CHECK(status IN ('RUNNING','PROPOSED','PARTIAL_VALIDATION','VALIDATION_FAILED','PROVIDER_FAILED','SUPERSEDED','NOT_NEEDED'));

ALTER TABLE accounting_analysis_runs ADD COLUMN failure_reason text;

COMMENT ON COLUMN validation_tasks.classification_run_id IS
 'Canonical run reviewed by this single aggregated accounting task.';
COMMENT ON COLUMN accounting_analysis_runs.failure_reason IS
 'Sanitized terminal workflow reason; never contains prompts or invoice content.';
