DROP TABLE IF EXISTS trace_hops;
DROP TABLE IF EXISTS trace_runs;
DROP TABLE IF EXISTS targets;

INSERT INTO app_metadata (key, value)
VALUES ('bootstrap_phase', 'phase_2')
ON CONFLICT(key) DO UPDATE SET
  value = excluded.value,
  updated_at = CURRENT_TIMESTAMP;