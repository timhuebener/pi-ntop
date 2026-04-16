DROP TABLE IF EXISTS trace_runs_1mo;
DROP TABLE IF EXISTS trace_runs_1w;
DROP TABLE IF EXISTS trace_runs_1d;
DROP TABLE IF EXISTS speed_tests_1mo;
DROP TABLE IF EXISTS speed_tests_1w;
DROP TABLE IF EXISTS speed_tests_1d;
DROP TABLE IF EXISTS interface_samples_1mo;
DROP TABLE IF EXISTS interface_samples_1w;
DROP TABLE IF EXISTS interface_samples_1d;

UPDATE app_metadata
SET value = 'phase_5', updated_at = CURRENT_TIMESTAMP
WHERE key = 'bootstrap_phase';
