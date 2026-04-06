DROP TABLE IF EXISTS interface_samples_1m;
DROP TABLE IF EXISTS alerts;

INSERT INTO app_metadata (key, value)
VALUES ('bootstrap_phase', 'phase_4')
ON CONFLICT(key) DO UPDATE SET
  value = excluded.value,
  updated_at = CURRENT_TIMESTAMP;