package monitor

import (
	"context"
	"fmt"
	"time"
)

func (s *Service) runRetentionLoop(ctx context.Context) error {
	interval := s.config.Retention.JobInterval
	if interval <= 0 {
		return nil
	}

	s.applyRetention(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.applyRetention(ctx)
		}
	}
}

func (s *Service) applyRetention(ctx context.Context) {
	status := s.currentRetentionSnapshot()
	now := time.Now().UTC()
	status.LastRunAt = now
	status.LastError = ""

	if err := s.applyRetentionOnce(ctx, now, &status); err != nil {
		status.LastError = err.Error()
		s.setRetentionSnapshot(status)
		return
	}

	status.LastSuccessAt = now
	s.setRetentionSnapshot(status)
}

func (s *Service) applyRetentionOnce(ctx context.Context, now time.Time, status *RetentionSnapshot) error {
	retention := s.config.Retention
	rollupWindow := retention.InterfaceRollup
	if retention.InterfaceRaw > 0 && (rollupWindow <= 0 || rollupWindow < retention.InterfaceRaw) {
		rollupWindow = retention.InterfaceRaw
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin retention transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if retention.InterfaceRaw > 0 {
		rawCutoff := formatTimestamp(now.Add(-retention.InterfaceRaw))
		rollupCutoff := formatTimestamp(now.Add(-rollupWindow))

		const rollupQuery = `
			INSERT INTO interface_samples_1m (
				interface_id,
				window_start,
				sample_count,
				avg_rx_bps,
				avg_tx_bps,
				peak_rx_bps,
				peak_tx_bps,
				rx_bytes_total,
				tx_bytes_total
			)
			SELECT
				interface_id,
				strftime('%Y-%m-%dT%H:%M:00Z', captured_at) AS window_start,
				COUNT(*),
				AVG(rx_bps),
				AVG(tx_bps),
				MAX(rx_bps),
				MAX(tx_bps),
				MAX(rx_bytes_total),
				MAX(tx_bytes_total)
			FROM interface_samples
			WHERE captured_at < ? AND captured_at >= ?
			GROUP BY interface_id, window_start
			ON CONFLICT(interface_id, window_start) DO UPDATE SET
				sample_count = excluded.sample_count,
				avg_rx_bps = excluded.avg_rx_bps,
				avg_tx_bps = excluded.avg_tx_bps,
				peak_rx_bps = excluded.peak_rx_bps,
				peak_tx_bps = excluded.peak_tx_bps,
				rx_bytes_total = excluded.rx_bytes_total,
				tx_bytes_total = excluded.tx_bytes_total;
		`

		result, err := tx.ExecContext(ctx, rollupQuery, rawCutoff, rollupCutoff)
		if err != nil {
			return fmt.Errorf("upsert interface rollups: %w", err)
		}
		status.UpsertedInterfaceRollups = rowsAffected(result)

		result, err = tx.ExecContext(ctx, `DELETE FROM interface_samples WHERE captured_at < ?;`, rawCutoff)
		if err != nil {
			return fmt.Errorf("delete raw interface samples: %w", err)
		}
		status.DeletedInterfaceSamples = rowsAffected(result)

		result, err = tx.ExecContext(ctx, `DELETE FROM interface_samples_1m WHERE window_start < ?;`, rollupCutoff)
		if err != nil {
			return fmt.Errorf("delete expired interface rollups: %w", err)
		}
		status.DeletedInterfaceRollups = rowsAffected(result)
	}

	if retention.TraceRuns > 0 {
		result, err := tx.ExecContext(ctx, `DELETE FROM trace_runs WHERE started_at < ?;`, formatTimestamp(now.Add(-retention.TraceRuns)))
		if err != nil {
			return fmt.Errorf("delete expired trace runs: %w", err)
		}
		status.DeletedTraceRuns = rowsAffected(result)
	}

	if retention.SpeedTests > 0 {
		result, err := tx.ExecContext(ctx, `DELETE FROM speed_tests WHERE started_at < ?;`, formatTimestamp(now.Add(-retention.SpeedTests)))
		if err != nil {
			return fmt.Errorf("delete expired speed tests: %w", err)
		}
		status.DeletedSpeedTests = rowsAffected(result)
	}

	if retention.ResolvedAlerts > 0 {
		cutoff := formatTimestamp(now.Add(-retention.ResolvedAlerts))
		result, err := tx.ExecContext(ctx, `DELETE FROM alerts WHERE status = 'resolved' AND resolved_at <> '' AND resolved_at < ?;`, cutoff)
		if err != nil {
			return fmt.Errorf("delete expired resolved alerts: %w", err)
		}
		status.DeletedResolvedAlerts = rowsAffected(result)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit retention transaction: %w", err)
	}
	committed = true
	return nil
}

func rowsAffected(result interface{ RowsAffected() (int64, error) }) int64 {
	count, err := result.RowsAffected()
	if err != nil {
		return 0
	}
	return count
}

func (s *Service) currentRetentionSnapshot() RetentionSnapshot {
	s.retentionMu.RLock()
	status := s.retentionState
	s.retentionMu.RUnlock()

	status.JobIntervalSeconds = durationSeconds(s.config.Retention.JobInterval)
	status.InterfaceRawSeconds = durationSeconds(s.config.Retention.InterfaceRaw)
	status.InterfaceRollupSeconds = durationSeconds(s.config.Retention.InterfaceRollup)
	status.TraceRetentionSeconds = durationSeconds(s.config.Retention.TraceRuns)
	status.SpeedRetentionSeconds = durationSeconds(s.config.Retention.SpeedTests)
	status.ResolvedAlertSeconds = durationSeconds(s.config.Retention.ResolvedAlerts)
	return status
}

func (s *Service) setRetentionSnapshot(status RetentionSnapshot) {
	s.retentionMu.Lock()
	s.retentionState = status
	s.retentionMu.Unlock()
}

func durationSeconds(value time.Duration) int {
	if value <= 0 {
		return 0
	}
	return int(value / time.Second)
}
