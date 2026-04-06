package monitor

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appdb "pi-ntop/internal/db"
)

func TestSpeedCollectorExecuteSpeedTest(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("0123456789abcdef0123456789abcdef"))
		case http.MethodPost:
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	collector := &SpeedCollector{
		client:        &http.Client{Timeout: 5 * time.Second},
		downloadBytes: 32,
		uploadBytes:   64,
	}

	measurement := collector.executeSpeedTest(context.Background(), speedTestTarget{
		Name:        "local",
		DownloadURL: server.URL,
		UploadURL:   server.URL,
	})

	if measurement.Status != "completed" {
		t.Fatalf("expected completed status, got %q", measurement.Status)
	}
	if measurement.DownloadBytes != 32 {
		t.Fatalf("expected 32 downloaded bytes, got %d", measurement.DownloadBytes)
	}
	if measurement.UploadBytes != 64 {
		t.Fatalf("expected 64 uploaded bytes, got %d", measurement.UploadBytes)
	}
	if measurement.DownloadBps <= 0 {
		t.Fatalf("expected positive download throughput, got %f", measurement.DownloadBps)
	}
	if measurement.UploadBps <= 0 {
		t.Fatalf("expected positive upload throughput, got %f", measurement.UploadBps)
	}
	if measurement.LatencyMs < 0 {
		t.Fatalf("expected non-negative latency, got %f", measurement.LatencyMs)
	}
}

func TestServiceListSpeedTargets(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	database := openTestDatabase(t, ctx)

	_, err := database.ExecContext(ctx, `
		INSERT INTO speed_test_targets (name, download_url, upload_url, enabled, interval_seconds)
		VALUES ('Cloudflare', 'https://example.test/down', 'https://example.test/up', 1, 180);
	`)
	if err != nil {
		t.Fatalf("insert speed target: %v", err)
	}

	_, err = database.ExecContext(ctx, `
		INSERT INTO speed_tests (target_id, started_at, completed_at, download_bps, upload_bps, latency_ms, download_bytes, upload_bytes, status, error_message)
		VALUES (1, '2026-04-06T12:00:00Z', '2026-04-06T12:00:05Z', 12000000, 3400000, 12.5, 5000000, 512000, 'completed', '');
	`)
	if err != nil {
		t.Fatalf("insert speed test: %v", err)
	}

	service := &Service{db: database}
	targets, err := service.listSpeedTargets(ctx, 8)
	if err != nil {
		t.Fatalf("list speed targets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected one speed target, got %d", len(targets))
	}
	if targets[0].LatestTest == nil {
		t.Fatal("expected latest speed test to be present")
	}
	if len(targets[0].History) != 1 {
		t.Fatalf("expected one history point, got %d", len(targets[0].History))
	}
	if !targets[0].IsHealthy {
		t.Fatal("expected target to be marked healthy")
	}
}

func openTestDatabase(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()

	database, err := appdb.Open(ctx, t.TempDir()+"/pi-ntop-test.sqlite")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	if err := appdb.RunMigrations(database); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	return database
}
