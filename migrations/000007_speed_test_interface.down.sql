PRAGMA foreign_keys = OFF;

BEGIN;

-- Recreate the original table without interface_name and with UNIQUE(download_url).
CREATE TABLE speed_test_targets_old (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  download_url TEXT NOT NULL UNIQUE,
  upload_url TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  interval_seconds INTEGER NOT NULL DEFAULT 180,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO speed_test_targets_old (id, name, download_url, upload_url, enabled, interval_seconds, created_at, updated_at)
  SELECT id, name, download_url, upload_url, enabled, interval_seconds, created_at, updated_at
  FROM speed_test_targets;

DROP TABLE speed_test_targets;
ALTER TABLE speed_test_targets_old RENAME TO speed_test_targets;

CREATE INDEX IF NOT EXISTS idx_speed_test_targets_enabled_name
  ON speed_test_targets (enabled, name ASC);

COMMIT;

PRAGMA foreign_keys = ON;
