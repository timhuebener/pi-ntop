-- Add optional interface binding to speed test targets.
-- interface_name defaults to '' (empty string) meaning "use default OS route".
-- Recreate the table to replace the UNIQUE(download_url) constraint with
-- UNIQUE(download_url, interface_name) so the same URL can be tested over
-- different network interfaces.

PRAGMA foreign_keys = OFF;

BEGIN;

CREATE TABLE speed_test_targets_new (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  download_url TEXT NOT NULL,
  upload_url TEXT NOT NULL DEFAULT '',
  interface_name TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  interval_seconds INTEGER NOT NULL DEFAULT 180,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(download_url, interface_name)
);

INSERT INTO speed_test_targets_new (id, name, download_url, upload_url, enabled, interval_seconds, created_at, updated_at)
  SELECT id, name, download_url, upload_url, enabled, interval_seconds, created_at, updated_at
  FROM speed_test_targets;

DROP TABLE speed_test_targets;
ALTER TABLE speed_test_targets_new RENAME TO speed_test_targets;

CREATE INDEX IF NOT EXISTS idx_speed_test_targets_enabled_name
  ON speed_test_targets (enabled, name ASC);

COMMIT;

PRAGMA foreign_keys = ON;
