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

	// ── Interface samples: raw → 1m → 1d → 1w → 1mo ──

	if retention.InterfaceRaw > 0 {
		rawCutoff := formatTimestamp(now.Add(-retention.InterfaceRaw))
		rollupCutoff := formatTimestamp(now.Add(-rollupWindow))

		const rollupQuery = `
			INSERT INTO interface_samples_1m (
				interface_id, window_start, sample_count,
				avg_rx_bps, avg_tx_bps, peak_rx_bps, peak_tx_bps,
				rx_bytes_total, tx_bytes_total
			)
			SELECT
				interface_id,
				strftime('%Y-%m-%dT%H:%M:00Z', captured_at) AS window_start,
				COUNT(*), AVG(rx_bps), AVG(tx_bps),
				MAX(rx_bps), MAX(tx_bps),
				MAX(rx_bytes_total), MAX(tx_bytes_total)
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
			return fmt.Errorf("upsert interface 1m rollups: %w", err)
		}
		status.UpsertedInterfaceRollups = rowsAffected(result)

		result, err = tx.ExecContext(ctx, `DELETE FROM interface_samples WHERE captured_at < ?;`, rawCutoff)
		if err != nil {
			return fmt.Errorf("delete raw interface samples: %w", err)
		}
		status.DeletedInterfaceSamples = rowsAffected(result)

		// 1m → 1d rollup
		rollup1mCutoff := formatTimestamp(now.Add(-rollupWindow))
		dailyCutoff := rollup1mCutoff
		if retention.InterfaceDaily > 0 {
			dailyCutoff = formatTimestamp(now.Add(-retention.InterfaceDaily))
		}

		const dailyRollupQuery = `
			INSERT INTO interface_samples_1d (
				interface_id, window_start, sample_count,
				avg_rx_bps, avg_tx_bps, peak_rx_bps, peak_tx_bps,
				rx_bytes_total, tx_bytes_total
			)
			SELECT
				interface_id,
				strftime('%Y-%m-%dT00:00:00Z', window_start) AS ws,
				SUM(sample_count), AVG(avg_rx_bps), AVG(avg_tx_bps),
				MAX(peak_rx_bps), MAX(peak_tx_bps),
				MAX(rx_bytes_total), MAX(tx_bytes_total)
			FROM interface_samples_1m
			WHERE window_start < ? AND window_start >= ?
			GROUP BY interface_id, ws
			ON CONFLICT(interface_id, window_start) DO UPDATE SET
				sample_count = excluded.sample_count,
				avg_rx_bps = excluded.avg_rx_bps,
				avg_tx_bps = excluded.avg_tx_bps,
				peak_rx_bps = excluded.peak_rx_bps,
				peak_tx_bps = excluded.peak_tx_bps,
				rx_bytes_total = excluded.rx_bytes_total,
				tx_bytes_total = excluded.tx_bytes_total;
		`

		result, err = tx.ExecContext(ctx, dailyRollupQuery, rollup1mCutoff, dailyCutoff)
		if err != nil {
			return fmt.Errorf("upsert interface daily rollups: %w", err)
		}
		status.UpsertedInterfaceDaily = rowsAffected(result)

		result, err = tx.ExecContext(ctx, `DELETE FROM interface_samples_1m WHERE window_start < ?;`, rollup1mCutoff)
		if err != nil {
			return fmt.Errorf("delete expired interface 1m rollups: %w", err)
		}
		status.DeletedInterfaceRollups = rowsAffected(result)

		// 1d → 1w rollup
		if retention.InterfaceDaily > 0 {
			weeklyCutoff := dailyCutoff
			if retention.InterfaceWeekly > 0 {
				weeklyCutoff = formatTimestamp(now.Add(-retention.InterfaceWeekly))
			}

			const weeklyRollupQuery = `
				INSERT INTO interface_samples_1w (
					interface_id, window_start, sample_count,
					avg_rx_bps, avg_tx_bps, peak_rx_bps, peak_tx_bps,
					rx_bytes_total, tx_bytes_total
				)
				SELECT
					interface_id,
					strftime('%Y-W%W', window_start) AS ws,
					SUM(sample_count), AVG(avg_rx_bps), AVG(avg_tx_bps),
					MAX(peak_rx_bps), MAX(peak_tx_bps),
					MAX(rx_bytes_total), MAX(tx_bytes_total)
				FROM interface_samples_1d
				WHERE window_start < ? AND window_start >= ?
				GROUP BY interface_id, ws
				ON CONFLICT(interface_id, window_start) DO UPDATE SET
					sample_count = excluded.sample_count,
					avg_rx_bps = excluded.avg_rx_bps,
					avg_tx_bps = excluded.avg_tx_bps,
					peak_rx_bps = excluded.peak_rx_bps,
					peak_tx_bps = excluded.peak_tx_bps,
					rx_bytes_total = excluded.rx_bytes_total,
					tx_bytes_total = excluded.tx_bytes_total;
			`

			result, err = tx.ExecContext(ctx, weeklyRollupQuery, dailyCutoff, weeklyCutoff)
			if err != nil {
				return fmt.Errorf("upsert interface weekly rollups: %w", err)
			}
			status.UpsertedInterfaceWeekly = rowsAffected(result)

			result, err = tx.ExecContext(ctx, `DELETE FROM interface_samples_1d WHERE window_start < ?;`, dailyCutoff)
			if err != nil {
				return fmt.Errorf("delete expired interface daily rollups: %w", err)
			}
			status.DeletedInterfaceDaily = rowsAffected(result)

			// 1w → 1mo rollup
			if retention.InterfaceWeekly > 0 {
				const monthlyRollupQuery = `
					INSERT INTO interface_samples_1mo (
						interface_id, window_start, sample_count,
						avg_rx_bps, avg_tx_bps, peak_rx_bps, peak_tx_bps,
						rx_bytes_total, tx_bytes_total
					)
					SELECT
						interface_id,
						strftime('%Y-%m', window_start) AS ws,
						SUM(sample_count), AVG(avg_rx_bps), AVG(avg_tx_bps),
						MAX(peak_rx_bps), MAX(peak_tx_bps),
						MAX(rx_bytes_total), MAX(tx_bytes_total)
					FROM interface_samples_1w
					WHERE window_start < ?
					GROUP BY interface_id, ws
					ON CONFLICT(interface_id, window_start) DO UPDATE SET
						sample_count = excluded.sample_count,
						avg_rx_bps = excluded.avg_rx_bps,
						avg_tx_bps = excluded.avg_tx_bps,
						peak_rx_bps = excluded.peak_rx_bps,
						peak_tx_bps = excluded.peak_tx_bps,
						rx_bytes_total = excluded.rx_bytes_total,
						tx_bytes_total = excluded.tx_bytes_total;
				`

				result, err = tx.ExecContext(ctx, monthlyRollupQuery, weeklyCutoff)
				if err != nil {
					return fmt.Errorf("upsert interface monthly rollups: %w", err)
				}
				status.UpsertedInterfaceMonthly = rowsAffected(result)

				result, err = tx.ExecContext(ctx, `DELETE FROM interface_samples_1w WHERE window_start < ?;`, weeklyCutoff)
				if err != nil {
					return fmt.Errorf("delete expired interface weekly rollups: %w", err)
				}
				status.DeletedInterfaceWeekly = rowsAffected(result)
			}
		}
	}

	// ── Speed tests: raw → 1d → 1w → 1mo ──

	if retention.SpeedTests > 0 {
		speedCutoff := formatTimestamp(now.Add(-retention.SpeedTests))
		speedDailyCutoff := speedCutoff
		if retention.SpeedDaily > 0 {
			speedDailyCutoff = formatTimestamp(now.Add(-retention.SpeedDaily))
		}

		const speedDailyQuery = `
			INSERT INTO speed_tests_1d (
				target_id, window_start, test_count,
				avg_download_bps, avg_upload_bps, avg_latency_ms,
				peak_download_bps, peak_upload_bps, fail_count
			)
			SELECT
				target_id,
				strftime('%Y-%m-%dT00:00:00Z', started_at) AS ws,
				COUNT(*),
				AVG(download_bps), AVG(upload_bps), AVG(latency_ms),
				MAX(download_bps), MAX(upload_bps),
				SUM(CASE WHEN status <> 'completed' THEN 1 ELSE 0 END)
			FROM speed_tests
			WHERE started_at < ? AND started_at >= ?
			GROUP BY target_id, ws
			ON CONFLICT(target_id, window_start) DO UPDATE SET
				test_count = excluded.test_count,
				avg_download_bps = excluded.avg_download_bps,
				avg_upload_bps = excluded.avg_upload_bps,
				avg_latency_ms = excluded.avg_latency_ms,
				peak_download_bps = excluded.peak_download_bps,
				peak_upload_bps = excluded.peak_upload_bps,
				fail_count = excluded.fail_count;
		`

		result, err := tx.ExecContext(ctx, speedDailyQuery, speedCutoff, speedDailyCutoff)
		if err != nil {
			return fmt.Errorf("upsert speed test daily rollups: %w", err)
		}
		status.UpsertedSpeedDaily = rowsAffected(result)

		result, err = tx.ExecContext(ctx, `DELETE FROM speed_tests WHERE started_at < ?;`, speedCutoff)
		if err != nil {
			return fmt.Errorf("delete expired speed tests: %w", err)
		}
		status.DeletedSpeedTests = rowsAffected(result)

		// 1d → 1w
		if retention.SpeedDaily > 0 {
			speedWeeklyCutoff := speedDailyCutoff
			if retention.SpeedWeekly > 0 {
				speedWeeklyCutoff = formatTimestamp(now.Add(-retention.SpeedWeekly))
			}

			const speedWeeklyQuery = `
				INSERT INTO speed_tests_1w (
					target_id, window_start, test_count,
					avg_download_bps, avg_upload_bps, avg_latency_ms,
					peak_download_bps, peak_upload_bps, fail_count
				)
				SELECT
					target_id,
					strftime('%Y-W%W', window_start) AS ws,
					SUM(test_count),
					AVG(avg_download_bps), AVG(avg_upload_bps), AVG(avg_latency_ms),
					MAX(peak_download_bps), MAX(peak_upload_bps),
					SUM(fail_count)
				FROM speed_tests_1d
				WHERE window_start < ? AND window_start >= ?
				GROUP BY target_id, ws
				ON CONFLICT(target_id, window_start) DO UPDATE SET
					test_count = excluded.test_count,
					avg_download_bps = excluded.avg_download_bps,
					avg_upload_bps = excluded.avg_upload_bps,
					avg_latency_ms = excluded.avg_latency_ms,
					peak_download_bps = excluded.peak_download_bps,
					peak_upload_bps = excluded.peak_upload_bps,
					fail_count = excluded.fail_count;
			`

			result, err = tx.ExecContext(ctx, speedWeeklyQuery, speedDailyCutoff, speedWeeklyCutoff)
			if err != nil {
				return fmt.Errorf("upsert speed test weekly rollups: %w", err)
			}
			status.UpsertedSpeedWeekly = rowsAffected(result)

			result, err = tx.ExecContext(ctx, `DELETE FROM speed_tests_1d WHERE window_start < ?;`, speedDailyCutoff)
			if err != nil {
				return fmt.Errorf("delete expired speed test daily rollups: %w", err)
			}
			status.DeletedSpeedDaily = rowsAffected(result)

			// 1w → 1mo
			if retention.SpeedWeekly > 0 {
				const speedMonthlyQuery = `
					INSERT INTO speed_tests_1mo (
						target_id, window_start, test_count,
						avg_download_bps, avg_upload_bps, avg_latency_ms,
						peak_download_bps, peak_upload_bps, fail_count
					)
					SELECT
						target_id,
						strftime('%Y-%m', window_start) AS ws,
						SUM(test_count),
						AVG(avg_download_bps), AVG(avg_upload_bps), AVG(avg_latency_ms),
						MAX(peak_download_bps), MAX(peak_upload_bps),
						SUM(fail_count)
					FROM speed_tests_1w
					WHERE window_start < ?
					GROUP BY target_id, ws
					ON CONFLICT(target_id, window_start) DO UPDATE SET
						test_count = excluded.test_count,
						avg_download_bps = excluded.avg_download_bps,
						avg_upload_bps = excluded.avg_upload_bps,
						avg_latency_ms = excluded.avg_latency_ms,
						peak_download_bps = excluded.peak_download_bps,
						peak_upload_bps = excluded.peak_upload_bps,
						fail_count = excluded.fail_count;
				`

				result, err = tx.ExecContext(ctx, speedMonthlyQuery, speedWeeklyCutoff)
				if err != nil {
					return fmt.Errorf("upsert speed test monthly rollups: %w", err)
				}
				status.UpsertedSpeedMonthly = rowsAffected(result)

				result, err = tx.ExecContext(ctx, `DELETE FROM speed_tests_1w WHERE window_start < ?;`, speedWeeklyCutoff)
				if err != nil {
					return fmt.Errorf("delete expired speed test weekly rollups: %w", err)
				}
				status.DeletedSpeedWeekly = rowsAffected(result)
			}
		}
	}

	// ── Trace runs: raw → 1d → 1w → 1mo ──

	if retention.TraceRuns > 0 {
		traceCutoff := formatTimestamp(now.Add(-retention.TraceRuns))
		traceDailyCutoff := traceCutoff
		if retention.TraceDaily > 0 {
			traceDailyCutoff = formatTimestamp(now.Add(-retention.TraceDaily))
		}

		const traceDailyQuery = `
			INSERT INTO trace_runs_1d (
				target_id, window_start, run_count,
				avg_hop_count, avg_degraded_hop_count,
				fail_count, route_change_count
			)
			SELECT
				target_id,
				strftime('%Y-%m-%dT00:00:00Z', started_at) AS ws,
				COUNT(*),
				AVG(hop_count), AVG(degraded_hop_count),
				SUM(CASE WHEN status <> 'completed' THEN 1 ELSE 0 END),
				SUM(route_changed)
			FROM trace_runs
			WHERE started_at < ? AND started_at >= ?
			GROUP BY target_id, ws
			ON CONFLICT(target_id, window_start) DO UPDATE SET
				run_count = excluded.run_count,
				avg_hop_count = excluded.avg_hop_count,
				avg_degraded_hop_count = excluded.avg_degraded_hop_count,
				fail_count = excluded.fail_count,
				route_change_count = excluded.route_change_count;
		`

		result, err := tx.ExecContext(ctx, traceDailyQuery, traceCutoff, traceDailyCutoff)
		if err != nil {
			return fmt.Errorf("upsert trace run daily rollups: %w", err)
		}
		status.UpsertedTraceDaily = rowsAffected(result)

		result, err = tx.ExecContext(ctx, `DELETE FROM trace_runs WHERE started_at < ?;`, traceCutoff)
		if err != nil {
			return fmt.Errorf("delete expired trace runs: %w", err)
		}
		status.DeletedTraceRuns = rowsAffected(result)

		// 1d → 1w
		if retention.TraceDaily > 0 {
			traceWeeklyCutoff := traceDailyCutoff
			if retention.TraceWeekly > 0 {
				traceWeeklyCutoff = formatTimestamp(now.Add(-retention.TraceWeekly))
			}

			const traceWeeklyQuery = `
				INSERT INTO trace_runs_1w (
					target_id, window_start, run_count,
					avg_hop_count, avg_degraded_hop_count,
					fail_count, route_change_count
				)
				SELECT
					target_id,
					strftime('%Y-W%W', window_start) AS ws,
					SUM(run_count),
					AVG(avg_hop_count), AVG(avg_degraded_hop_count),
					SUM(fail_count), SUM(route_change_count)
				FROM trace_runs_1d
				WHERE window_start < ? AND window_start >= ?
				GROUP BY target_id, ws
				ON CONFLICT(target_id, window_start) DO UPDATE SET
					run_count = excluded.run_count,
					avg_hop_count = excluded.avg_hop_count,
					avg_degraded_hop_count = excluded.avg_degraded_hop_count,
					fail_count = excluded.fail_count,
					route_change_count = excluded.route_change_count;
			`

			result, err = tx.ExecContext(ctx, traceWeeklyQuery, traceDailyCutoff, traceWeeklyCutoff)
			if err != nil {
				return fmt.Errorf("upsert trace run weekly rollups: %w", err)
			}
			status.UpsertedTraceWeekly = rowsAffected(result)

			result, err = tx.ExecContext(ctx, `DELETE FROM trace_runs_1d WHERE window_start < ?;`, traceDailyCutoff)
			if err != nil {
				return fmt.Errorf("delete expired trace run daily rollups: %w", err)
			}
			status.DeletedTraceDaily = rowsAffected(result)

			// 1w → 1mo
			if retention.TraceWeekly > 0 {
				const traceMonthlyQuery = `
					INSERT INTO trace_runs_1mo (
						target_id, window_start, run_count,
						avg_hop_count, avg_degraded_hop_count,
						fail_count, route_change_count
					)
					SELECT
						target_id,
						strftime('%Y-%m', window_start) AS ws,
						SUM(run_count),
						AVG(avg_hop_count), AVG(avg_degraded_hop_count),
						SUM(fail_count), SUM(route_change_count)
					FROM trace_runs_1w
					WHERE window_start < ?
					GROUP BY target_id, ws
					ON CONFLICT(target_id, window_start) DO UPDATE SET
						run_count = excluded.run_count,
						avg_hop_count = excluded.avg_hop_count,
						avg_degraded_hop_count = excluded.avg_degraded_hop_count,
						fail_count = excluded.fail_count,
						route_change_count = excluded.route_change_count;
				`

				result, err = tx.ExecContext(ctx, traceMonthlyQuery, traceWeeklyCutoff)
				if err != nil {
					return fmt.Errorf("upsert trace run monthly rollups: %w", err)
				}
				status.UpsertedTraceMonthly = rowsAffected(result)

				result, err = tx.ExecContext(ctx, `DELETE FROM trace_runs_1w WHERE window_start < ?;`, traceWeeklyCutoff)
				if err != nil {
					return fmt.Errorf("delete expired trace run weekly rollups: %w", err)
				}
				status.DeletedTraceWeekly = rowsAffected(result)
			}
		}
	}

	// ── Resolved alerts ──

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
	status.InterfaceDailySeconds = durationSeconds(s.config.Retention.InterfaceDaily)
	status.InterfaceWeeklySeconds = durationSeconds(s.config.Retention.InterfaceWeekly)
	status.TraceRetentionSeconds = durationSeconds(s.config.Retention.TraceRuns)
	status.TraceDailySeconds = durationSeconds(s.config.Retention.TraceDaily)
	status.TraceWeeklySeconds = durationSeconds(s.config.Retention.TraceWeekly)
	status.SpeedRetentionSeconds = durationSeconds(s.config.Retention.SpeedTests)
	status.SpeedDailySeconds = durationSeconds(s.config.Retention.SpeedDaily)
	status.SpeedWeeklySeconds = durationSeconds(s.config.Retention.SpeedWeekly)
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
