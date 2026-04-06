package monitor

import (
	"context"
	"testing"
	"time"

	"pi-ntop/internal/config"
)

func TestServiceEvaluateAlerts(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	database := openTestDatabase(t, ctx)

	_, err := database.ExecContext(ctx, `
		INSERT INTO speed_test_targets (id, name, download_url, upload_url, enabled, interval_seconds)
		VALUES (1, 'Cloudflare', 'https://example.test/down', 'https://example.test/up', 1, 180);
	`)
	if err != nil {
		t.Fatalf("insert speed target: %v", err)
	}

	_, err = database.ExecContext(ctx, `
		INSERT INTO speed_tests (target_id, started_at, completed_at, download_bps, upload_bps, latency_ms, download_bytes, upload_bytes, status, error_message)
		VALUES (1, '2026-04-06T12:00:00Z', '2026-04-06T12:00:05Z', 1000000, 250000, 225, 5000000, 512000, 'completed', '');
	`)
	if err != nil {
		t.Fatalf("insert speed test: %v", err)
	}

	_, err = database.ExecContext(ctx, `
		INSERT INTO targets (id, name, host, enabled, probe_interval_seconds)
		VALUES (1, 'Edge path', '1.1.1.1', 1, 45);
	`)
	if err != nil {
		t.Fatalf("insert trace target: %v", err)
	}

	_, err = database.ExecContext(ctx, `
		INSERT INTO trace_runs (id, target_id, started_at, completed_at, hop_count, status, route_fingerprint, route_changed, degraded_hop_count, error_message)
		VALUES (1, 1, '2026-04-06T12:00:00Z', '2026-04-06T12:00:02Z', 2, 'completed', 'a>b', 1, 1, '');
	`)
	if err != nil {
		t.Fatalf("insert trace run: %v", err)
	}

	_, err = database.ExecContext(ctx, `
		INSERT INTO trace_hops (trace_run_id, hop_index, address, hostname, avg_rtt_ms, jitter_ms, loss_pct, is_timeout)
		VALUES
			(1, 1, '192.0.2.1', 'edge-router', 20, 2, 0, 0),
			(1, 2, '198.51.100.2', 'wan-hop', 190, 5, 12, 0);
	`)
	if err != nil {
		t.Fatalf("insert trace hops: %v", err)
	}

	service := &Service{
		db: database,
		config: config.Config{
			Alerts: config.AlertThresholds{
				MinDownloadBps: 5_000_000,
				MaxLatencyMs:   150,
				MaxLossPct:     5,
			},
		},
	}

	if err := service.evaluateAlerts(ctx); err != nil {
		t.Fatalf("evaluate alerts: %v", err)
	}

	alerts, total, err := service.listActiveAlerts(ctx, 10)
	if err != nil {
		t.Fatalf("list active alerts: %v", err)
	}
	if total != 5 {
		t.Fatalf("expected 5 active alerts, got %d", total)
	}
	if len(alerts) != 5 {
		t.Fatalf("expected 5 returned alerts, got %d", len(alerts))
	}

	_, err = database.ExecContext(ctx, `
		INSERT INTO speed_tests (target_id, started_at, completed_at, download_bps, upload_bps, latency_ms, download_bytes, upload_bytes, status, error_message)
		VALUES (1, '2026-04-06T12:10:00Z', '2026-04-06T12:10:05Z', 12000000, 3400000, 15, 5000000, 512000, 'completed', '');
	`)
	if err != nil {
		t.Fatalf("insert healthy speed test: %v", err)
	}

	_, err = database.ExecContext(ctx, `
		INSERT INTO trace_runs (id, target_id, started_at, completed_at, hop_count, status, route_fingerprint, route_changed, degraded_hop_count, error_message)
		VALUES (2, 1, '2026-04-06T12:10:00Z', '2026-04-06T12:10:02Z', 2, 'completed', 'a>b', 0, 0, '');
	`)
	if err != nil {
		t.Fatalf("insert healthy trace run: %v", err)
	}

	_, err = database.ExecContext(ctx, `
		INSERT INTO trace_hops (trace_run_id, hop_index, address, hostname, avg_rtt_ms, jitter_ms, loss_pct, is_timeout)
		VALUES
			(2, 1, '192.0.2.1', 'edge-router', 18, 1, 0, 0),
			(2, 2, '198.51.100.2', 'wan-hop', 35, 3, 0, 0);
	`)
	if err != nil {
		t.Fatalf("insert healthy trace hops: %v", err)
	}

	if err := service.evaluateAlerts(ctx); err != nil {
		t.Fatalf("re-evaluate alerts: %v", err)
	}

	_, total, err = service.listActiveAlerts(ctx, 10)
	if err != nil {
		t.Fatalf("list active alerts after resolve: %v", err)
	}
	if total != 0 {
		t.Fatalf("expected 0 active alerts after recovery, got %d", total)
	}
}

func TestServiceApplyRetention(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	database := openTestDatabase(t, ctx)
	now := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)

	_, err := database.ExecContext(ctx, `
		INSERT INTO interfaces (id, name, display_name, is_active, is_loopback, last_seen_at, created_at, updated_at)
		VALUES (1, 'en0', 'en0', 1, 0, ?, ?, ?);
	`, formatTimestamp(now), formatTimestamp(now), formatTimestamp(now))
	if err != nil {
		t.Fatalf("insert interface: %v", err)
	}

	_, err = database.ExecContext(ctx, `
		INSERT INTO interface_samples (interface_id, captured_at, rx_bytes_total, tx_bytes_total, rx_bps, tx_bps)
		VALUES
			(1, ?, 1000, 500, 10000, 2000),
			(1, ?, 2000, 1000, 15000, 3000);
	`, formatTimestamp(now.Add(-2*time.Hour)), formatTimestamp(now.Add(-30*time.Minute)))
	if err != nil {
		t.Fatalf("insert interface samples: %v", err)
	}

	_, err = database.ExecContext(ctx, `
		INSERT INTO targets (id, name, host, enabled, probe_interval_seconds)
		VALUES (1, 'Edge path', '1.1.1.1', 1, 45);
		INSERT INTO trace_runs (id, target_id, started_at, completed_at, hop_count, status, route_fingerprint, route_changed, degraded_hop_count, error_message)
		VALUES
			(1, 1, ?, ?, 2, 'completed', 'old', 0, 0, ''),
			(2, 1, ?, ?, 2, 'completed', 'new', 0, 0, '');
	`, formatTimestamp(now.Add(-48*time.Hour)), formatTimestamp(now.Add(-48*time.Hour+2*time.Second)), formatTimestamp(now.Add(-2*time.Hour)), formatTimestamp(now.Add(-2*time.Hour+2*time.Second)))
	if err != nil {
		t.Fatalf("insert trace data: %v", err)
	}

	_, err = database.ExecContext(ctx, `
		INSERT INTO speed_test_targets (id, name, download_url, upload_url, enabled, interval_seconds)
		VALUES (1, 'Cloudflare', 'https://example.test/down', '', 1, 180);
		INSERT INTO speed_tests (id, target_id, started_at, completed_at, download_bps, upload_bps, latency_ms, download_bytes, upload_bytes, status, error_message)
		VALUES
			(1, 1, ?, ?, 1000000, 0, 120, 5000000, 0, 'completed', ''),
			(2, 1, ?, ?, 12000000, 0, 20, 5000000, 0, 'completed', '');
	`, formatTimestamp(now.Add(-40*24*time.Hour)), formatTimestamp(now.Add(-40*24*time.Hour+2*time.Second)), formatTimestamp(now.Add(-2*time.Hour)), formatTimestamp(now.Add(-2*time.Hour+2*time.Second)))
	if err != nil {
		t.Fatalf("insert speed data: %v", err)
	}

	_, err = database.ExecContext(ctx, `
		INSERT INTO alerts (dedupe_key, created_at, updated_at, last_seen_at, resolved_at, severity, source_type, source_id, source_name, metric_key, status, threshold_value, current_value, message)
		VALUES
			('old-alert', ?, ?, ?, ?, 'warning', 'speed_target', 1, 'Cloudflare', 'latency_ms', 'resolved', 150, 200, 'old'),
			('new-alert', ?, ?, ?, ?, 'warning', 'speed_target', 1, 'Cloudflare', 'latency_ms', 'resolved', 150, 170, 'new');
	`, formatTimestamp(now.Add(-10*24*time.Hour)), formatTimestamp(now.Add(-10*24*time.Hour)), formatTimestamp(now.Add(-10*24*time.Hour)), formatTimestamp(now.Add(-10*24*time.Hour)), formatTimestamp(now.Add(-6*time.Hour)), formatTimestamp(now.Add(-6*time.Hour)), formatTimestamp(now.Add(-6*time.Hour)), formatTimestamp(now.Add(-6*time.Hour)))
	if err != nil {
		t.Fatalf("insert alerts: %v", err)
	}

	service := &Service{
		db: database,
		config: config.Config{
			Retention: config.RetentionPolicy{
				InterfaceRaw:    time.Hour,
				InterfaceRollup: 7 * 24 * time.Hour,
				TraceRuns:       24 * time.Hour,
				SpeedTests:      30 * 24 * time.Hour,
				ResolvedAlerts:  24 * time.Hour,
			},
		},
	}

	status := service.currentRetentionSnapshot()
	status.LastRunAt = now
	if err := service.applyRetentionOnce(ctx, now, &status); err != nil {
		t.Fatalf("apply retention: %v", err)
	}

	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM interface_samples_1m;`).Scan(&count); err != nil {
		t.Fatalf("count interface rollups: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 rollup row, got %d", count)
	}

	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM interface_samples;`).Scan(&count); err != nil {
		t.Fatalf("count interface samples: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 retained raw interface sample, got %d", count)
	}

	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM trace_runs;`).Scan(&count); err != nil {
		t.Fatalf("count trace runs: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 retained trace run, got %d", count)
	}

	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM speed_tests;`).Scan(&count); err != nil {
		t.Fatalf("count speed tests: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 retained speed test, got %d", count)
	}

	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM alerts;`).Scan(&count); err != nil {
		t.Fatalf("count alerts: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 retained resolved alert, got %d", count)
	}

	if status.UpsertedInterfaceRollups == 0 {
		t.Fatal("expected rollup upsert count to be recorded")
	}
	if status.DeletedInterfaceSamples == 0 || status.DeletedTraceRuns == 0 || status.DeletedSpeedTests == 0 || status.DeletedResolvedAlerts == 0 {
		t.Fatalf("expected retention cleanup counts to be recorded, got %+v", status)
	}
}
