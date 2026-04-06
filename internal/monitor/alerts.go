package monitor

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"pi-ntop/internal/config"
)

type desiredAlert struct {
	DedupeKey      string
	Severity       string
	SourceType     string
	SourceID       int64
	SourceName     string
	MetricKey      string
	ThresholdValue float64
	CurrentValue   float64
	Message        string
}

func (s *Service) runAlertLoop(ctx context.Context) error {
	interval := s.config.Alerts.EvaluateInterval
	if interval <= 0 {
		return nil
	}

	if err := s.evaluateAlerts(ctx); err != nil {
		return err
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.evaluateAlerts(ctx); err != nil {
				return err
			}
		}
	}
}

func (s *Service) evaluateAlerts(ctx context.Context) error {
	pathTargets, err := s.listPathTargets(ctx, 1)
	if err != nil {
		return fmt.Errorf("load path targets for alerts: %w", err)
	}

	speedTargets, err := s.listSpeedTargets(ctx, 1)
	if err != nil {
		return fmt.Errorf("load speed targets for alerts: %w", err)
	}

	desired := buildDesiredAlerts(pathTargets, speedTargets, s.config.Alerts)
	if err := s.syncAlerts(ctx, desired); err != nil {
		return fmt.Errorf("sync alerts: %w", err)
	}

	return nil
}

func buildDesiredAlerts(pathTargets []TargetPathSnapshot, speedTargets []SpeedTargetSnapshot, thresholds config.AlertThresholds) []desiredAlert {
	alerts := make([]desiredAlert, 0, len(pathTargets)*3+len(speedTargets)*3)

	for _, target := range speedTargets {
		if target.LatestTest == nil {
			continue
		}

		if target.LatestTest.Status == "failed" {
			alerts = append(alerts, desiredAlert{
				DedupeKey:      fmt.Sprintf("speed_target:%d:availability", target.ID),
				Severity:       "critical",
				SourceType:     "speed_target",
				SourceID:       target.ID,
				SourceName:     target.Name,
				MetricKey:      "availability",
				ThresholdValue: 1,
				CurrentValue:   0,
				Message:        fmt.Sprintf("Latest speed test for %s failed: %s", target.Name, strings.TrimSpace(target.LatestTest.ErrorMessage)),
			})
		}

		if thresholds.MinDownloadBps > 0 && target.LatestTest.DownloadBps > 0 && target.LatestTest.DownloadBps < thresholds.MinDownloadBps {
			severity := "warning"
			if target.LatestTest.DownloadBps < thresholds.MinDownloadBps/2 {
				severity = "critical"
			}
			alerts = append(alerts, desiredAlert{
				DedupeKey:      fmt.Sprintf("speed_target:%d:download_bps", target.ID),
				Severity:       severity,
				SourceType:     "speed_target",
				SourceID:       target.ID,
				SourceName:     target.Name,
				MetricKey:      "download_bps",
				ThresholdValue: thresholds.MinDownloadBps,
				CurrentValue:   target.LatestTest.DownloadBps,
				Message:        fmt.Sprintf("Download throughput for %s fell below threshold: %s < %s", target.Name, formatBitrateValue(target.LatestTest.DownloadBps), formatBitrateValue(thresholds.MinDownloadBps)),
			})
		}

		if thresholds.MaxLatencyMs > 0 && target.LatestTest.LatencyMs > thresholds.MaxLatencyMs {
			severity := "warning"
			if target.LatestTest.LatencyMs >= thresholds.MaxLatencyMs*2 {
				severity = "critical"
			}
			alerts = append(alerts, desiredAlert{
				DedupeKey:      fmt.Sprintf("speed_target:%d:latency_ms", target.ID),
				Severity:       severity,
				SourceType:     "speed_target",
				SourceID:       target.ID,
				SourceName:     target.Name,
				MetricKey:      "latency_ms",
				ThresholdValue: thresholds.MaxLatencyMs,
				CurrentValue:   target.LatestTest.LatencyMs,
				Message:        fmt.Sprintf("Latency for %s exceeded threshold: %s > %s", target.Name, formatAlertValue("latency_ms", target.LatestTest.LatencyMs), formatAlertValue("latency_ms", thresholds.MaxLatencyMs)),
			})
		}
	}

	for _, target := range pathTargets {
		if target.LatestRun == nil {
			continue
		}

		if target.RouteChanged {
			alerts = append(alerts, desiredAlert{
				DedupeKey:      fmt.Sprintf("path_target:%d:route_changed", target.ID),
				Severity:       "warning",
				SourceType:     "path_target",
				SourceID:       target.ID,
				SourceName:     target.Name,
				MetricKey:      "route_changed",
				ThresholdValue: 0,
				CurrentValue:   1,
				Message:        fmt.Sprintf("Route changed for %s on the latest traceroute run", target.Name),
			})
		}

		if thresholds.MaxLatencyMs > 0 {
			worstLatencyHop := TraceHopSnapshot{}
			for _, hop := range target.LatestRun.Hops {
				if hop.AvgRTTMs > worstLatencyHop.AvgRTTMs {
					worstLatencyHop = hop
				}
			}
			if worstLatencyHop.AvgRTTMs > thresholds.MaxLatencyMs {
				severity := "warning"
				if worstLatencyHop.AvgRTTMs >= thresholds.MaxLatencyMs*2 {
					severity = "critical"
				}
				hopLabel := worstLatencyHop.Address
				if hopLabel == "" {
					hopLabel = fmt.Sprintf("hop %d", worstLatencyHop.HopIndex)
				}
				alerts = append(alerts, desiredAlert{
					DedupeKey:      fmt.Sprintf("path_target:%d:latency_ms", target.ID),
					Severity:       severity,
					SourceType:     "path_target",
					SourceID:       target.ID,
					SourceName:     target.Name,
					MetricKey:      "latency_ms",
					ThresholdValue: thresholds.MaxLatencyMs,
					CurrentValue:   worstLatencyHop.AvgRTTMs,
					Message:        fmt.Sprintf("Path latency for %s exceeded threshold at %s: %s > %s", target.Name, hopLabel, formatAlertValue("latency_ms", worstLatencyHop.AvgRTTMs), formatAlertValue("latency_ms", thresholds.MaxLatencyMs)),
				})
			}
		}

		if thresholds.MaxLossPct > 0 {
			worstLossHop := TraceHopSnapshot{}
			for _, hop := range target.LatestRun.Hops {
				if hop.LossPct > worstLossHop.LossPct {
					worstLossHop = hop
				}
			}
			if worstLossHop.LossPct > thresholds.MaxLossPct {
				severity := "warning"
				if worstLossHop.LossPct >= thresholds.MaxLossPct*2 {
					severity = "critical"
				}
				hopLabel := worstLossHop.Address
				if hopLabel == "" {
					hopLabel = fmt.Sprintf("hop %d", worstLossHop.HopIndex)
				}
				alerts = append(alerts, desiredAlert{
					DedupeKey:      fmt.Sprintf("path_target:%d:loss_pct", target.ID),
					Severity:       severity,
					SourceType:     "path_target",
					SourceID:       target.ID,
					SourceName:     target.Name,
					MetricKey:      "loss_pct",
					ThresholdValue: thresholds.MaxLossPct,
					CurrentValue:   worstLossHop.LossPct,
					Message:        fmt.Sprintf("Packet loss for %s exceeded threshold at %s: %s > %s", target.Name, hopLabel, formatAlertValue("loss_pct", worstLossHop.LossPct), formatAlertValue("loss_pct", thresholds.MaxLossPct)),
				})
			}
		}
	}

	return alerts
}

func (s *Service) syncAlerts(ctx context.Context, desired []desiredAlert) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin alerts transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	activeKeys, err := listActiveAlertKeys(ctx, tx)
	if err != nil {
		return err
	}

	now := formatTimestamp(time.Now().UTC())
	const upsertQuery = `
		INSERT INTO alerts (
			dedupe_key,
			created_at,
			updated_at,
			last_seen_at,
			severity,
			source_type,
			source_id,
			source_name,
			metric_key,
			status,
			threshold_value,
			current_value,
			message
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?, ?)
		ON CONFLICT(dedupe_key) DO UPDATE SET
			updated_at = excluded.updated_at,
			last_seen_at = excluded.last_seen_at,
			resolved_at = '',
			severity = excluded.severity,
			source_type = excluded.source_type,
			source_id = excluded.source_id,
			source_name = excluded.source_name,
			metric_key = excluded.metric_key,
			status = 'active',
			threshold_value = excluded.threshold_value,
			current_value = excluded.current_value,
			message = excluded.message;
	`

	wanted := make(map[string]struct{}, len(desired))
	for _, alert := range desired {
		wanted[alert.DedupeKey] = struct{}{}
		if _, err := tx.ExecContext(
			ctx,
			upsertQuery,
			alert.DedupeKey,
			now,
			now,
			now,
			alert.Severity,
			alert.SourceType,
			alert.SourceID,
			alert.SourceName,
			alert.MetricKey,
			alert.ThresholdValue,
			alert.CurrentValue,
			alert.Message,
		); err != nil {
			return fmt.Errorf("upsert alert %s: %w", alert.DedupeKey, err)
		}
	}

	const resolveQuery = `
		UPDATE alerts
		SET status = 'resolved', updated_at = ?, resolved_at = ?
		WHERE dedupe_key = ? AND status = 'active';
	`

	for _, dedupeKey := range activeKeys {
		if _, ok := wanted[dedupeKey]; ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, resolveQuery, now, now, dedupeKey); err != nil {
			return fmt.Errorf("resolve alert %s: %w", dedupeKey, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alerts transaction: %w", err)
	}
	committed = true
	return nil
}

func listActiveAlertKeys(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT dedupe_key FROM alerts WHERE status = 'active';`)
	if err != nil {
		return nil, fmt.Errorf("query active alert keys: %w", err)
	}
	defer rows.Close()

	keys := make([]string, 0)
	for rows.Next() {
		var dedupeKey string
		if err := rows.Scan(&dedupeKey); err != nil {
			return nil, fmt.Errorf("scan active alert key: %w", err)
		}
		keys = append(keys, dedupeKey)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active alert keys: %w", err)
	}

	return keys, nil
}

func (s *Service) listActiveAlerts(ctx context.Context, limit int) ([]AlertSnapshot, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM alerts WHERE status = 'active';`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count active alerts: %w", err)
	}

	if limit <= 0 || total == 0 {
		return []AlertSnapshot{}, total, nil
	}

	const query = `
		SELECT id, severity, source_type, source_id, source_name, metric_key, status, message,
		       threshold_value, current_value, created_at, updated_at, last_seen_at, resolved_at
		FROM alerts
		WHERE status = 'active'
		ORDER BY CASE severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END, updated_at DESC
		LIMIT ?;
	`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("query active alerts: %w", err)
	}
	defer rows.Close()

	alerts := make([]AlertSnapshot, 0, limit)
	for rows.Next() {
		var (
			alert         AlertSnapshot
			createdAtRaw  string
			updatedAtRaw  string
			lastSeenAtRaw string
			resolvedAtRaw string
		)

		if err := rows.Scan(
			&alert.ID,
			&alert.Severity,
			&alert.SourceType,
			&alert.SourceID,
			&alert.SourceName,
			&alert.MetricKey,
			&alert.Status,
			&alert.Message,
			&alert.ThresholdValue,
			&alert.CurrentValue,
			&createdAtRaw,
			&updatedAtRaw,
			&lastSeenAtRaw,
			&resolvedAtRaw,
		); err != nil {
			return nil, 0, fmt.Errorf("scan active alert: %w", err)
		}

		createdAt, err := parseTimestamp(createdAtRaw)
		if err != nil {
			return nil, 0, fmt.Errorf("parse alert created_at: %w", err)
		}
		updatedAt, err := parseTimestamp(updatedAtRaw)
		if err != nil {
			return nil, 0, fmt.Errorf("parse alert updated_at: %w", err)
		}
		lastSeenAt, err := parseTimestamp(lastSeenAtRaw)
		if err != nil {
			return nil, 0, fmt.Errorf("parse alert last_seen_at: %w", err)
		}

		alert.CreatedAt = createdAt
		alert.UpdatedAt = updatedAt
		alert.LastSeenAt = lastSeenAt
		if resolvedAtRaw != "" {
			resolvedAt, err := parseTimestamp(resolvedAtRaw)
			if err != nil {
				return nil, 0, fmt.Errorf("parse alert resolved_at: %w", err)
			}
			alert.ResolvedAt = resolvedAt
		}

		alerts = append(alerts, alert)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate active alerts: %w", err)
	}

	return alerts, total, nil
}

func formatAlertValue(metricKey string, value float64) string {
	switch metricKey {
	case "download_bps":
		return formatBitrateValue(value)
	case "latency_ms":
		return fmt.Sprintf("%.1f ms", value)
	case "loss_pct":
		return fmt.Sprintf("%.1f%%", value)
	default:
		return fmt.Sprintf("%.2f", value)
	}
}

func formatBitrateValue(bitsPerSecond float64) string {
	units := []string{"bps", "Kbps", "Mbps", "Gbps", "Tbps"}
	value := bitsPerSecond
	unitIndex := 0
	for value >= 1000 && unitIndex < len(units)-1 {
		value /= 1000
		unitIndex++
	}

	if unitIndex == 0 {
		return fmt.Sprintf("%.0f %s", value, units[unitIndex])
	}
	return fmt.Sprintf("%.1f %s", value, units[unitIndex])
}
