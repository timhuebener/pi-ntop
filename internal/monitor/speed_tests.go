package monitor

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"pi-ntop/internal/config"
)

const defaultSpeedIntervalSeconds = 180

type SpeedCollector struct {
	db                *sql.DB
	configuredTargets []config.SpeedTestTarget
	defaultInterval   time.Duration
	requestTimeout    time.Duration
	downloadBytes     int64
	uploadBytes       int64
	lastRun           map[int64]time.Time
	client            *http.Client
}

type SpeedTargetSnapshot struct {
	ID              int64              `json:"id"`
	Name            string             `json:"name"`
	DownloadURL     string             `json:"downloadUrl"`
	UploadURL       string             `json:"uploadUrl"`
	IntervalSeconds int                `json:"intervalSeconds"`
	Status          string             `json:"status"`
	LatestTest      *SpeedTestSnapshot `json:"latestTest"`
	History         []SpeedTestPoint   `json:"history"`
	HasUpload       bool               `json:"hasUpload"`
	IsHealthy       bool               `json:"isHealthy"`
}

type SpeedTestSnapshot struct {
	ID            int64     `json:"id"`
	StartedAt     time.Time `json:"startedAt"`
	CompletedAt   time.Time `json:"completedAt"`
	DownloadBps   float64   `json:"downloadBps"`
	UploadBps     float64   `json:"uploadBps"`
	LatencyMs     float64   `json:"latencyMs"`
	DownloadBytes int64     `json:"downloadBytes"`
	UploadBytes   int64     `json:"uploadBytes"`
	Status        string    `json:"status"`
	ErrorMessage  string    `json:"errorMessage"`
}

type SpeedTestPoint struct {
	StartedAt   time.Time `json:"startedAt"`
	DownloadBps float64   `json:"downloadBps"`
	UploadBps   float64   `json:"uploadBps"`
	LatencyMs   float64   `json:"latencyMs"`
	Status      string    `json:"status"`
}

type speedTestTarget struct {
	ID              int64
	Name            string
	DownloadURL     string
	UploadURL       string
	IntervalSeconds int
}

type speedTestMeasurement struct {
	DownloadBps   float64
	UploadBps     float64
	LatencyMs     float64
	DownloadBytes int64
	UploadBytes   int64
	Status        string
	ErrorMessage  string
}

type latestSpeedRow struct {
	TargetID        int64
	Name            string
	DownloadURL     string
	UploadURL       string
	IntervalSeconds int
	TestID          sql.NullInt64
	StartedAt       sql.NullString
	CompletedAt     sql.NullString
	DownloadBps     sql.NullFloat64
	UploadBps       sql.NullFloat64
	LatencyMs       sql.NullFloat64
	DownloadBytes   sql.NullInt64
	UploadBytes     sql.NullInt64
	Status          sql.NullString
	ErrorMessage    sql.NullString
}

func NewSpeedCollector(database *sql.DB, targets []config.SpeedTestTarget, defaultInterval, requestTimeout time.Duration, downloadBytes, uploadBytes int64) *SpeedCollector {
	if defaultInterval <= 0 {
		defaultInterval = defaultSpeedIntervalSeconds * time.Second
	}
	if requestTimeout <= 0 {
		requestTimeout = 45 * time.Second
	}
	if downloadBytes <= 0 {
		downloadBytes = 5_000_000
	}
	if uploadBytes < 0 {
		uploadBytes = 0
	}

	return &SpeedCollector{
		db:                database,
		configuredTargets: append([]config.SpeedTestTarget(nil), targets...),
		defaultInterval:   defaultInterval,
		requestTimeout:    requestTimeout,
		downloadBytes:     downloadBytes,
		uploadBytes:       uploadBytes,
		lastRun:           make(map[int64]time.Time),
		client: &http.Client{
			Timeout: requestTimeout,
		},
	}
}

func (c *SpeedCollector) Run(ctx context.Context) error {
	if err := c.syncConfiguredTargets(ctx); err != nil {
		return err
	}

	if len(c.configuredTargets) == 0 {
		return nil
	}

	c.collect(ctx)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			c.collect(ctx)
		}
	}
}

func (c *SpeedCollector) collect(ctx context.Context) {
	if err := c.collectDue(ctx); err != nil {
		// speed test failures are persisted with per-target status; only scheduler/database errors reach here.
		log.Printf("collect speed tests: %v", err)
	}
}

func (c *SpeedCollector) collectDue(ctx context.Context) error {
	targets, err := c.listEnabledTargets(ctx)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, target := range targets {
		interval := time.Duration(target.IntervalSeconds) * time.Second
		if interval <= 0 {
			interval = c.defaultInterval
		}

		lastRunAt := c.lastRun[target.ID]
		if !lastRunAt.IsZero() && now.Sub(lastRunAt) < interval {
			continue
		}

		c.lastRun[target.ID] = now
		if err := c.collectTarget(ctx, target); err != nil {
			return err
		}
	}

	return nil
}

func (c *SpeedCollector) collectTarget(ctx context.Context, target speedTestTarget) error {
	startedAt := time.Now().UTC()
	testCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	measurement := c.executeSpeedTest(testCtx, target)
	if err := c.persistSpeedTest(ctx, target.ID, startedAt, time.Now().UTC(), measurement); err != nil {
		return fmt.Errorf("persist speed test: %w", err)
	}

	return nil
}

func (c *SpeedCollector) executeSpeedTest(ctx context.Context, target speedTestTarget) speedTestMeasurement {
	measurement := speedTestMeasurement{Status: "completed"}
	errors := make([]string, 0, 2)
	attemptedChecks := 0
	successfulChecks := 0

	attemptedChecks++
	downloadBps, downloadBytes, latencyMs, err := c.measureDownload(ctx, target.DownloadURL, c.downloadBytes)
	if err != nil {
		errors = append(errors, "download: "+err.Error())
	} else {
		measurement.DownloadBps = downloadBps
		measurement.DownloadBytes = downloadBytes
		measurement.LatencyMs = latencyMs
		successfulChecks++
	}

	if target.UploadURL != "" {
		attemptedChecks++
		uploadBps, uploadBytes, err := c.measureUpload(ctx, target.UploadURL, c.uploadBytes)
		if err != nil {
			errors = append(errors, "upload: "+err.Error())
		} else {
			measurement.UploadBps = uploadBps
			measurement.UploadBytes = uploadBytes
			successfulChecks++
		}
	}

	measurement.Status = deriveSpeedStatus(attemptedChecks, successfulChecks)
	measurement.ErrorMessage = strings.Join(errors, "; ")
	return measurement
}

func (c *SpeedCollector) measureDownload(ctx context.Context, rawURL string, maxBytes int64) (float64, int64, float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Cache-Control", "no-cache")

	startedAt := time.Now()
	resp, err := c.client.Do(req)
	latency := time.Since(startedAt)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("request download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return 0, 0, latency.Seconds() * 1000, fmt.Errorf("unexpected status %s", resp.Status)
	}

	readStartedAt := time.Now()
	var bytesRead int64
	if maxBytes > 0 {
		bytesRead, err = io.CopyN(io.Discard, resp.Body, maxBytes)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			err = nil
		}
	} else {
		bytesRead, err = io.Copy(io.Discard, resp.Body)
	}
	if err != nil {
		return 0, bytesRead, latency.Seconds() * 1000, fmt.Errorf("read body: %w", err)
	}

	return throughputBps(bytesRead, time.Since(readStartedAt)), bytesRead, latency.Seconds() * 1000, nil
}

func (c *SpeedCollector) measureUpload(ctx context.Context, rawURL string, size int64) (float64, int64, error) {
	if size <= 0 {
		return 0, 0, nil
	}

	payload := bytes.Repeat([]byte("0123456789abcdef"), int((size/16)+1))
	payload = payload[:size]

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(payload))
	if err != nil {
		return 0, 0, fmt.Errorf("build request: %w", err)
	}
	req.ContentLength = int64(len(payload))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Cache-Control", "no-cache")

	startedAt := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("request upload: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= http.StatusBadRequest {
		return 0, 0, fmt.Errorf("unexpected status %s", resp.Status)
	}

	return throughputBps(int64(len(payload)), time.Since(startedAt)), int64(len(payload)), nil
}

func throughputBps(bytesTransferred int64, duration time.Duration) float64 {
	if bytesTransferred <= 0 || duration <= 0 {
		return 0
	}
	return (float64(bytesTransferred) * 8) / duration.Seconds()
}

func deriveSpeedStatus(attemptedChecks, successfulChecks int) string {
	if attemptedChecks == 0 || successfulChecks == 0 {
		return "failed"
	}
	if successfulChecks == attemptedChecks {
		return "completed"
	}
	return "partial"
}

func (c *SpeedCollector) syncConfiguredTargets(ctx context.Context) error {
	if len(c.configuredTargets) == 0 {
		return nil
	}

	defaultIntervalSeconds := int(c.defaultInterval / time.Second)
	if defaultIntervalSeconds <= 0 {
		defaultIntervalSeconds = defaultSpeedIntervalSeconds
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin speed target sync transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	const query = `
		INSERT INTO speed_test_targets (name, download_url, upload_url, enabled, interval_seconds)
		VALUES (?, ?, ?, 1, ?)
		ON CONFLICT(download_url) DO UPDATE SET
			name = excluded.name,
			upload_url = excluded.upload_url,
			enabled = 1,
			updated_at = CURRENT_TIMESTAMP;
	`

	for _, target := range c.configuredTargets {
		if _, err := tx.ExecContext(ctx, query, target.Name, target.DownloadURL, target.UploadURL, defaultIntervalSeconds); err != nil {
			return fmt.Errorf("upsert speed target %s: %w", target.DownloadURL, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit speed target sync: %w", err)
	}
	committed = true
	return nil
}

func (c *SpeedCollector) listEnabledTargets(ctx context.Context) ([]speedTestTarget, error) {
	const query = `
		SELECT id, name, download_url, upload_url, interval_seconds
		FROM speed_test_targets
		WHERE enabled = 1
		ORDER BY name ASC, download_url ASC;
	`

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query speed test targets: %w", err)
	}
	defer rows.Close()

	var targets []speedTestTarget
	for rows.Next() {
		var target speedTestTarget
		if err := rows.Scan(&target.ID, &target.Name, &target.DownloadURL, &target.UploadURL, &target.IntervalSeconds); err != nil {
			return nil, fmt.Errorf("scan speed test target: %w", err)
		}
		targets = append(targets, target)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate speed test targets: %w", err)
	}

	return targets, nil
}

func (c *SpeedCollector) persistSpeedTest(ctx context.Context, targetID int64, startedAt, completedAt time.Time, measurement speedTestMeasurement) error {
	const query = `
		INSERT INTO speed_tests (
			target_id,
			started_at,
			completed_at,
			download_bps,
			upload_bps,
			latency_ms,
			download_bytes,
			upload_bytes,
			status,
			error_message
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`

	if _, err := c.db.ExecContext(
		ctx,
		query,
		targetID,
		formatTimestamp(startedAt),
		formatTimestamp(completedAt),
		measurement.DownloadBps,
		measurement.UploadBps,
		measurement.LatencyMs,
		measurement.DownloadBytes,
		measurement.UploadBytes,
		measurement.Status,
		measurement.ErrorMessage,
	); err != nil {
		return fmt.Errorf("insert speed test for id %d: %w", targetID, err)
	}

	return nil
}

func (s *Service) listSpeedTargets(ctx context.Context, historyLimit int) ([]SpeedTargetSnapshot, error) {
	latestRows, err := s.listLatestSpeedRows(ctx)
	if err != nil {
		return nil, err
	}

	historyByTarget, err := s.listSpeedHistory(ctx, historyLimit)
	if err != nil {
		return nil, err
	}

	targets := make([]SpeedTargetSnapshot, 0, len(latestRows))
	for _, row := range latestRows {
		target := SpeedTargetSnapshot{
			ID:              row.TargetID,
			Name:            row.Name,
			DownloadURL:     row.DownloadURL,
			UploadURL:       row.UploadURL,
			IntervalSeconds: row.IntervalSeconds,
			Status:          "pending",
			History:         historyByTarget[row.TargetID],
			HasUpload:       row.UploadURL != "",
		}

		if row.TestID.Valid {
			startedAt, err := parseTimestamp(row.StartedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse speed test start: %w", err)
			}

			completedAt := startedAt
			if row.CompletedAt.Valid {
				completedAt, err = parseTimestamp(row.CompletedAt.String)
				if err != nil {
					return nil, fmt.Errorf("parse speed test completion: %w", err)
				}
			}

			target.LatestTest = &SpeedTestSnapshot{
				ID:            row.TestID.Int64,
				StartedAt:     startedAt,
				CompletedAt:   completedAt,
				DownloadBps:   row.DownloadBps.Float64,
				UploadBps:     row.UploadBps.Float64,
				LatencyMs:     row.LatencyMs.Float64,
				DownloadBytes: row.DownloadBytes.Int64,
				UploadBytes:   row.UploadBytes.Int64,
				Status:        row.Status.String,
				ErrorMessage:  row.ErrorMessage.String,
			}
			target.Status = target.LatestTest.Status
			target.IsHealthy = target.Status == "completed"
		}

		targets = append(targets, target)
	}

	return targets, nil
}

func (s *Service) listLatestSpeedRows(ctx context.Context) ([]latestSpeedRow, error) {
	const query = `
		SELECT
			t.id,
			t.name,
			t.download_url,
			t.upload_url,
			t.interval_seconds,
			s.id,
			s.started_at,
			s.completed_at,
			s.download_bps,
			s.upload_bps,
			s.latency_ms,
			s.download_bytes,
			s.upload_bytes,
			s.status,
			s.error_message
		FROM speed_test_targets t
		LEFT JOIN speed_tests s ON s.id = (
			SELECT s2.id
			FROM speed_tests s2
			WHERE s2.target_id = t.id
			ORDER BY s2.started_at DESC
			LIMIT 1
		)
		WHERE t.enabled = 1
		ORDER BY t.name ASC, t.download_url ASC;
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query latest speed tests: %w", err)
	}
	defer rows.Close()

	var latest []latestSpeedRow
	for rows.Next() {
		var row latestSpeedRow
		if err := rows.Scan(
			&row.TargetID,
			&row.Name,
			&row.DownloadURL,
			&row.UploadURL,
			&row.IntervalSeconds,
			&row.TestID,
			&row.StartedAt,
			&row.CompletedAt,
			&row.DownloadBps,
			&row.UploadBps,
			&row.LatencyMs,
			&row.DownloadBytes,
			&row.UploadBytes,
			&row.Status,
			&row.ErrorMessage,
		); err != nil {
			return nil, fmt.Errorf("scan latest speed test: %w", err)
		}
		latest = append(latest, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate latest speed tests: %w", err)
	}

	return latest, nil
}

func (s *Service) listSpeedHistory(ctx context.Context, limit int) (map[int64][]SpeedTestPoint, error) {
	if limit <= 0 {
		return map[int64][]SpeedTestPoint{}, nil
	}

	const query = `
		SELECT target_id, started_at, download_bps, upload_bps, latency_ms, status
		FROM (
			SELECT
				target_id,
				started_at,
				download_bps,
				upload_bps,
				latency_ms,
				status,
				ROW_NUMBER() OVER (PARTITION BY target_id ORDER BY started_at DESC) AS row_num
			FROM speed_tests
		)
		WHERE row_num <= ?
		ORDER BY target_id ASC, started_at ASC;
	`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query speed history: %w", err)
	}
	defer rows.Close()

	history := make(map[int64][]SpeedTestPoint)
	for rows.Next() {
		var (
			targetID     int64
			startedAtRaw string
			point        SpeedTestPoint
		)

		if err := rows.Scan(&targetID, &startedAtRaw, &point.DownloadBps, &point.UploadBps, &point.LatencyMs, &point.Status); err != nil {
			return nil, fmt.Errorf("scan speed history: %w", err)
		}

		startedAt, err := parseTimestamp(startedAtRaw)
		if err != nil {
			return nil, fmt.Errorf("parse speed history timestamp: %w", err)
		}

		point.StartedAt = startedAt
		history[targetID] = append(history[targetID], point)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate speed history: %w", err)
	}

	return history, nil
}
